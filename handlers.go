package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-sql-driver/mysql"
)

type app struct {
	db *sql.DB
}

const (
	formatoFecha      = "2006-01-02"
	maxNombreCat      = 80
	maxCuerpo         = 1 << 20 // 1 MB
	tiempoMaxConsulta = 10 * time.Second
	errMySQLDuplicado = 1062
)

// --- Respuestas JSON ---

func escribirJSON(w http.ResponseWriter, codigo int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(codigo)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		log.Printf("error escribiendo JSON: %v", err)
	}
}

func escribirError(w http.ResponseWriter, codigo int, mensaje string) {
	escribirJSON(w, codigo, map[string]string{"error": mensaje})
}

// errorInterno registra el detalle técnico y responde un 500 genérico.
func errorInterno(w http.ResponseWriter, r *http.Request, err error) {
	log.Printf("ERROR %s %s: %v", r.Method, r.URL.Path, err)
	escribirError(w, http.StatusInternalServerError, "error interno al consultar la base de datos")
}

func ctxConsulta(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), tiempoMaxConsulta)
}

func idDeRuta(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

func fechaTexto(t time.Time) string { return t.Format(formatoFecha) }

// --- Reglas de negocio puras ---

// clasificarVencimiento devuelve el nivel de alerta de un lote según los días
// que le quedan antes de vencer:
//   - "vencido"  si ya venció (días < 0)
//   - "liquidar" si está dentro de la ventana de liquidación (días <= diasLiquidacion)
//   - "devolver" en otro caso (aún hay tiempo de devolverlo al proveedor)
func clasificarVencimiento(diasRestantes, diasLiquidacion int) string {
	switch {
	case diasRestantes < 0:
		return "vencido"
	case diasRestantes <= diasLiquidacion:
		return "liquidar"
	default:
		return "devolver"
	}
}

// validarNombreCategoria limpia espacios y aplica las reglas de longitud.
func validarNombreCategoria(nombre string) (string, string) {
	nombre = strings.TrimSpace(nombre)
	switch {
	case nombre == "":
		return "", "el nombre es obligatorio"
	case utf8.RuneCountInString(nombre) > maxNombreCat:
		return "", "el nombre no puede superar 80 caracteres"
	}
	return nombre, ""
}

// escaparLike evita que % y _ escritos por el usuario actúen como comodines.
func escaparLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

var tiposEtiqueta = map[string]bool{
	"necesidad": true, "ingrediente": true, "contraindicacion": true, "otro": true,
}

// --- Modelos ---

type categoria struct {
	ID     int64  `json:"id"`
	Nombre string `json:"nombre"`
}

type productoResumen struct {
	ID           int64   `json:"id"`
	CodigoBarras *string `json:"codigo_barras"`
	Nombre       string  `json:"nombre"`
	Marca        *string `json:"marca"`
	Presentacion *string `json:"presentacion"`
	CategoriaID  int64   `json:"categoria_id"`
	Categoria    string  `json:"categoria"`
	PrecioVenta  float64 `json:"precio_venta"`
	EsGranel     bool    `json:"es_granel"`
	Stock        int64   `json:"stock"`
	StockMinimo  int64   `json:"stock_minimo"`
	StockBajo    bool    `json:"stock_bajo"`
}

type proveedor struct {
	ID               int64   `json:"id"`
	Nombre           string  `json:"nombre"`
	Telefono         *string `json:"telefono"`
	AceptaDevolucion bool    `json:"acepta_devolucion"`
}

type fichaProducto struct {
	productoResumen
	DiasAlertaTemprana    int64      `json:"dias_alerta_temprana"`
	DiasAlertaLiquidacion int64      `json:"dias_alerta_liquidacion"`
	ParaQueSirve          *string    `json:"para_que_sirve"`
	ModoToma              *string    `json:"modo_toma"`
	Contraindicaciones    *string    `json:"contraindicaciones"`
	Ingredientes          *string    `json:"ingredientes"`
	AvisoComplemento      string     `json:"aviso_complemento"`
	Proveedor             *proveedor `json:"proveedor"`
}

type etiqueta struct {
	ID     int64  `json:"id"`
	Nombre string `json:"nombre"`
	Tipo   string `json:"tipo"`
}

type etiquetaConConteo struct {
	etiqueta
	CantidadProductos int64 `json:"cantidad_productos"`
}

type loteFEFO struct {
	ID               int64   `json:"id"`
	CodigoLote       *string `json:"codigo_lote"`
	FechaVencimiento string  `json:"fecha_vencimiento"`
	CantidadActual   int64   `json:"cantidad_actual"`
	DiasRestantes    int64   `json:"dias_restantes"`
}

type alertaVencimiento struct {
	LoteID                int64   `json:"lote_id"`
	CodigoLote            *string `json:"codigo_lote"`
	ProductoID            int64   `json:"producto_id"`
	Producto              string  `json:"producto"`
	Marca                 *string `json:"marca"`
	Presentacion          *string `json:"presentacion"`
	FechaVencimiento      string  `json:"fecha_vencimiento"`
	CantidadActual        int64   `json:"cantidad_actual"`
	DiasRestantes         int64   `json:"dias_restantes"`
	DiasAlertaLiquidacion int64   `json:"dias_alerta_liquidacion"`
	Nivel                 string  `json:"nivel"`
	Proveedor             *string `json:"proveedor"`
	ProveedorTelefono     *string `json:"proveedor_telefono"`
	AceptaDevolucion      *bool   `json:"acepta_devolucion"`
}

// --- Consultas compartidas ---

// stockVigente: solo cuentan los lotes no vencidos.
const stockVigente = `COALESCE(SUM(CASE WHEN l.fecha_vencimiento >= CURRENT_DATE THEN l.cantidad_actual ELSE 0 END), 0)`

const selectProductoResumen = `
SELECT p.id, p.codigo_barras, p.nombre, p.marca, p.presentacion,
       c.id, c.nombre, p.precio_venta, p.es_granel, p.stock_minimo,
       ` + stockVigente + ` AS stock
FROM productos p
JOIN categorias c ON c.id = p.categoria_id
LEFT JOIN lotes l ON l.producto_id = p.id
`

const agruparProductoResumen = ` GROUP BY p.id, c.id, c.nombre `

func escanearProductos(rows *sql.Rows) ([]productoResumen, error) {
	defer rows.Close()
	lista := []productoResumen{}
	for rows.Next() {
		var p productoResumen
		if err := rows.Scan(&p.ID, &p.CodigoBarras, &p.Nombre, &p.Marca, &p.Presentacion,
			&p.CategoriaID, &p.Categoria, &p.PrecioVenta, &p.EsGranel, &p.StockMinimo, &p.Stock); err != nil {
			return nil, err
		}
		p.StockBajo = p.Stock <= p.StockMinimo
		lista = append(lista, p)
	}
	return lista, rows.Err()
}

// --- Handlers ---

// GET /api/salud
func (a *app) salud(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	var version, base string
	if err := a.db.QueryRowContext(ctx, `SELECT VERSION(), DATABASE()`).Scan(&version, &base); err != nil {
		log.Printf("salud: MySQL no responde: %v", err)
		escribirJSON(w, http.StatusInternalServerError, map[string]string{
			"api": "ok", "error": "sin conexión con MySQL",
		})
		return
	}
	escribirJSON(w, http.StatusOK, map[string]string{
		"api":           "ok",
		"mysql":         version,
		"base":          base,
		"hora_servidor": time.Now().Format(time.RFC3339),
	})
}

// GET /api/categorias
func (a *app) listarCategorias(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	rows, err := a.db.QueryContext(ctx, `SELECT id, nombre FROM categorias ORDER BY nombre`)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	defer rows.Close()
	lista := []categoria{}
	for rows.Next() {
		var c categoria
		if err := rows.Scan(&c.ID, &c.Nombre); err != nil {
			errorInterno(w, r, err)
			return
		}
		lista = append(lista, c)
	}
	if err := rows.Err(); err != nil {
		errorInterno(w, r, err)
		return
	}
	escribirJSON(w, http.StatusOK, lista)
}

// POST /api/categorias  {"nombre":"..."}
func (a *app) crearCategoria(w http.ResponseWriter, r *http.Request) {
	var cuerpo struct {
		Nombre *string `json:"nombre"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCuerpo))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cuerpo); err != nil {
		msg := "JSON inválido; se espera {\"nombre\":\"...\"}"
		if errors.Is(err, io.EOF) {
			msg = "el cuerpo está vacío; se espera {\"nombre\":\"...\"}"
		}
		escribirError(w, http.StatusBadRequest, msg)
		return
	}
	if dec.More() {
		escribirError(w, http.StatusBadRequest, "el cuerpo debe contener un solo objeto JSON")
		return
	}
	if cuerpo.Nombre == nil {
		escribirError(w, http.StatusBadRequest, "el nombre es obligatorio")
		return
	}
	nombre, problema := validarNombreCategoria(*cuerpo.Nombre)
	if problema != "" {
		escribirError(w, http.StatusBadRequest, problema)
		return
	}

	ctx, cancel := ctxConsulta(r)
	defer cancel()
	res, err := a.db.ExecContext(ctx, `INSERT INTO categorias (nombre) VALUES (?)`, nombre)
	if err != nil {
		var me *mysql.MySQLError
		if errors.As(err, &me) && me.Number == errMySQLDuplicado {
			escribirError(w, http.StatusConflict, "ya existe una categoría con ese nombre")
			return
		}
		errorInterno(w, r, err)
		return
	}
	id, err := res.LastInsertId()
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/categorias/"+strconv.FormatInt(id, 10))
	escribirJSON(w, http.StatusCreated, categoria{ID: id, Nombre: nombre})
}

// GET /api/productos?q=texto
func (a *app) listarProductos(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	patron := "%" + escaparLike(q) + "%"

	ctx, cancel := ctxConsulta(r)
	defer cancel()
	rows, err := a.db.QueryContext(ctx, selectProductoResumen+`
WHERE p.activo = 1
  AND (? = '' OR p.nombre LIKE ? OR p.marca LIKE ? OR p.codigo_barras LIKE ?)`+
		agruparProductoResumen+`ORDER BY p.nombre, p.id`,
		q, patron, patron, patron)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	lista, err := escanearProductos(rows)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	escribirJSON(w, http.StatusOK, lista)
}

// GET /api/productos/{id}
func (a *app) detalleProducto(w http.ResponseWriter, r *http.Request) {
	id, ok := idDeRuta(r)
	if !ok {
		escribirError(w, http.StatusBadRequest, "el id debe ser un número entero positivo")
		return
	}
	ctx, cancel := ctxConsulta(r)
	defer cancel()

	var f fichaProducto
	var provID *int64
	var provNombre, provTel *string
	var provDevol *bool
	err := a.db.QueryRowContext(ctx, `
SELECT p.id, p.codigo_barras, p.nombre, p.marca, p.presentacion,
       c.id, c.nombre, p.precio_venta, p.es_granel, p.stock_minimo,
       `+stockVigente+` AS stock,
       p.dias_alerta_temprana, p.dias_alerta_liquidacion,
       p.para_que_sirve, p.modo_toma, p.contraindicaciones, p.ingredientes, p.aviso_complemento,
       pr.id, pr.nombre, pr.telefono, pr.acepta_devolucion
FROM productos p
JOIN categorias c ON c.id = p.categoria_id
LEFT JOIN proveedores pr ON pr.id = p.proveedor_id
LEFT JOIN lotes l ON l.producto_id = p.id
WHERE p.id = ? AND p.activo = 1
GROUP BY p.id, c.id, c.nombre, pr.id, pr.nombre, pr.telefono, pr.acepta_devolucion`, id).Scan(
		&f.ID, &f.CodigoBarras, &f.Nombre, &f.Marca, &f.Presentacion,
		&f.CategoriaID, &f.Categoria, &f.PrecioVenta, &f.EsGranel, &f.StockMinimo,
		&f.Stock,
		&f.DiasAlertaTemprana, &f.DiasAlertaLiquidacion,
		&f.ParaQueSirve, &f.ModoToma, &f.Contraindicaciones, &f.Ingredientes, &f.AvisoComplemento,
		&provID, &provNombre, &provTel, &provDevol)
	if errors.Is(err, sql.ErrNoRows) {
		escribirError(w, http.StatusNotFound, "producto no encontrado")
		return
	}
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	f.StockBajo = f.Stock <= f.StockMinimo
	if provID != nil {
		f.Proveedor = &proveedor{ID: *provID, Nombre: *provNombre, Telefono: provTel, AceptaDevolucion: provDevol != nil && *provDevol}
	}

	// Etiquetas activas del producto.
	etiquetas := []etiqueta{}
	rows, err := a.db.QueryContext(ctx, `
SELECT e.id, e.nombre, e.tipo
FROM etiquetas e
JOIN producto_etiqueta pe ON pe.etiqueta_id = e.id
WHERE pe.producto_id = ? AND e.activa = 1
ORDER BY e.tipo, e.nombre`, id)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var e etiqueta
		if err := rows.Scan(&e.ID, &e.Nombre, &e.Tipo); err != nil {
			errorInterno(w, r, err)
			return
		}
		etiquetas = append(etiquetas, e)
	}
	if err := rows.Err(); err != nil {
		errorInterno(w, r, err)
		return
	}

	// Lotes vendibles en orden FEFO (primero el que vence primero).
	lotes := []loteFEFO{}
	rowsL, err := a.db.QueryContext(ctx, `
SELECT id, codigo_lote, fecha_vencimiento, cantidad_actual,
       DATEDIFF(fecha_vencimiento, CURRENT_DATE) AS dias_restantes
FROM lotes
WHERE producto_id = ? AND cantidad_actual > 0 AND fecha_vencimiento >= CURRENT_DATE
ORDER BY fecha_vencimiento, id`, id)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	defer rowsL.Close()
	for rowsL.Next() {
		var l loteFEFO
		var venc time.Time
		if err := rowsL.Scan(&l.ID, &l.CodigoLote, &venc, &l.CantidadActual, &l.DiasRestantes); err != nil {
			errorInterno(w, r, err)
			return
		}
		l.FechaVencimiento = fechaTexto(venc)
		lotes = append(lotes, l)
	}
	if err := rowsL.Err(); err != nil {
		errorInterno(w, r, err)
		return
	}

	escribirJSON(w, http.StatusOK, map[string]any{
		"ficha":     f,
		"etiquetas": etiquetas,
		"lotes":     lotes,
	})
}

// GET /api/etiquetas?tipo=necesidad|contraindicacion|ingrediente|otro
func (a *app) listarEtiquetas(w http.ResponseWriter, r *http.Request) {
	tipo := strings.TrimSpace(r.URL.Query().Get("tipo"))
	if tipo != "" && !tiposEtiqueta[tipo] {
		escribirError(w, http.StatusBadRequest, "tipo inválido; use necesidad, contraindicacion, ingrediente u otro")
		return
	}
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	rows, err := a.db.QueryContext(ctx, `
SELECT e.id, e.nombre, e.tipo, COUNT(p.id) AS cantidad_productos
FROM etiquetas e
LEFT JOIN producto_etiqueta pe ON pe.etiqueta_id = e.id
LEFT JOIN productos p ON p.id = pe.producto_id AND p.activo = 1
WHERE e.activa = 1 AND (? = '' OR e.tipo = ?)
GROUP BY e.id, e.nombre, e.tipo
ORDER BY e.tipo, e.nombre`, tipo, tipo)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	defer rows.Close()
	lista := []etiquetaConConteo{}
	for rows.Next() {
		var e etiquetaConConteo
		if err := rows.Scan(&e.ID, &e.Nombre, &e.Tipo, &e.CantidadProductos); err != nil {
			errorInterno(w, r, err)
			return
		}
		lista = append(lista, e)
	}
	if err := rows.Err(); err != nil {
		errorInterno(w, r, err)
		return
	}
	escribirJSON(w, http.StatusOK, lista)
}

// GET /api/etiquetas/{id}/productos
func (a *app) productosPorEtiqueta(w http.ResponseWriter, r *http.Request) {
	id, ok := idDeRuta(r)
	if !ok {
		escribirError(w, http.StatusBadRequest, "el id debe ser un número entero positivo")
		return
	}
	ctx, cancel := ctxConsulta(r)
	defer cancel()

	var e etiqueta
	err := a.db.QueryRowContext(ctx,
		`SELECT id, nombre, tipo FROM etiquetas WHERE id = ? AND activa = 1`, id).Scan(&e.ID, &e.Nombre, &e.Tipo)
	if errors.Is(err, sql.ErrNoRows) {
		escribirError(w, http.StatusNotFound, "etiqueta no encontrada")
		return
	}
	if err != nil {
		errorInterno(w, r, err)
		return
	}

	rows, err := a.db.QueryContext(ctx, selectProductoResumen+`
JOIN producto_etiqueta pe ON pe.producto_id = p.id
WHERE p.activo = 1 AND pe.etiqueta_id = ?`+
		agruparProductoResumen+`ORDER BY (stock > 0) DESC, p.nombre, p.id`, id)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	lista, err := escanearProductos(rows)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	escribirJSON(w, http.StatusOK, map[string]any{"etiqueta": e, "productos": lista})
}

// GET /api/alertas/stock-bajo
func (a *app) alertasStockBajo(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	rows, err := a.db.QueryContext(ctx, selectProductoResumen+`
WHERE p.activo = 1`+agruparProductoResumen+`
HAVING stock <= p.stock_minimo
ORDER BY stock, p.nombre, p.id`)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	lista, err := escanearProductos(rows)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	escribirJSON(w, http.StatusOK, lista)
}

// GET /api/alertas/vencimientos
func (a *app) alertasVencimientos(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	rows, err := a.db.QueryContext(ctx, `
SELECT l.id, l.codigo_lote, p.id, p.nombre, p.marca, p.presentacion,
       l.fecha_vencimiento, l.cantidad_actual,
       DATEDIFF(l.fecha_vencimiento, CURRENT_DATE) AS dias_restantes,
       p.dias_alerta_liquidacion,
       pr.nombre, pr.telefono, pr.acepta_devolucion
FROM lotes l
JOIN productos p ON p.id = l.producto_id
LEFT JOIN proveedores pr ON pr.id = p.proveedor_id
WHERE p.activo = 1
  AND l.cantidad_actual > 0
  AND l.fecha_vencimiento <= CURRENT_DATE + INTERVAL p.dias_alerta_temprana DAY
ORDER BY l.fecha_vencimiento, l.id`)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	defer rows.Close()
	lista := []alertaVencimiento{}
	for rows.Next() {
		var al alertaVencimiento
		var venc time.Time
		if err := rows.Scan(&al.LoteID, &al.CodigoLote, &al.ProductoID, &al.Producto, &al.Marca, &al.Presentacion,
			&venc, &al.CantidadActual, &al.DiasRestantes, &al.DiasAlertaLiquidacion,
			&al.Proveedor, &al.ProveedorTelefono, &al.AceptaDevolucion); err != nil {
			errorInterno(w, r, err)
			return
		}
		al.FechaVencimiento = fechaTexto(venc)
		al.Nivel = clasificarVencimiento(int(al.DiasRestantes), int(al.DiasAlertaLiquidacion))
		lista = append(lista, al)
	}
	if err := rows.Err(); err != nil {
		errorInterno(w, r, err)
		return
	}
	escribirJSON(w, http.StatusOK, lista)
}
