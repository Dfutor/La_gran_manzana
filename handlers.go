package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-sql-driver/mysql"
)

type app struct {
	db          *sql.DB
	docsUsuario string
	docsClave   string
}

const (
	formatoFecha       = "2006-01-02"
	maxNombreCat       = 80
	maxBusqueda        = 100
	maxCantidadFEFO    = 1_000_000
	maxEtiquetasEvitar = 20
	maxCuerpo          = 1 << 20 // 1 MB
	tiempoMaxConsulta  = 10 * time.Second

	errMySQLDuplicado    = 1062
	errMySQLFKReferencia = 1451

	avisoOrientativo = "Información orientativa (Decreto 3249 de 2006). Los productos son complementos, no medicamentos: ante algo serio, consulte a su médico."
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

// errorInterno registra el detalle técnico y responde un 500 genérico con la
// referencia de la petición, para poder ubicarla en el log.
func errorInterno(w http.ResponseWriter, r *http.Request, err error) {
	ref := idPeticion(r.Context())
	log.Printf("ERROR [%s] %s %s: %v", ref, r.Method, r.URL.Path, err)
	escribirError(w, http.StatusInternalServerError, "error interno al consultar la base de datos (ref "+ref+")")
}

func ctxConsulta(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), tiempoMaxConsulta)
}

func idDeRuta(r *http.Request) (int64, bool) {
	return parsearIDPositivo(r.PathValue("id"))
}

func parsearIDPositivo(s string) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return id, err == nil && id > 0
}

func fechaTexto(t time.Time) string { return t.Format(formatoFecha) }

// --- Reglas de negocio puras (sin base de datos, cubiertas por pruebas unitarias) ---

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

// sugerirAccion traduce el nivel de alerta en la acción que debe tomar la tienda.
// Los textos corresponden a los valores de bajas.accion_previa del esquema.
func sugerirAccion(nivel string, aceptaDevolucion bool) string {
	switch nivel {
	case "vencido":
		return "Retirar del estante y registrar la baja por vencimiento"
	case "liquidar":
		return "Liquidar: rebaja, combo o venta destacada"
	case "devolver":
		if aceptaDevolucion {
			return "Gestionar la devolución con el distribuidor"
		}
		return "El proveedor no acepta devolución: hacer venta destacada"
	default:
		return ""
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

// parsearListaIDs convierte "11, 13,11" en [11 13]: sin repetidos, todos > 0.
func parsearListaIDs(s string, max int) ([]int64, error) {
	ids := []int64{}
	if strings.TrimSpace(s) == "" {
		return ids, nil
	}
	vistos := map[int64]bool{}
	for _, parte := range strings.Split(s, ",") {
		id, ok := parsearIDPositivo(parte)
		if !ok {
			return nil, fmt.Errorf("%q no es un id válido", strings.TrimSpace(parte))
		}
		if !vistos[id] {
			vistos[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) > max {
		return nil, fmt.Errorf("máximo %d ids", max)
	}
	return ids, nil
}

// placeholders devuelve "?,?,?" para n valores (nunca concatena valores).
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// planificarFEFO reparte la cantidad pedida entre los lotes, empezando por el que
// vence primero. Ignora lotes vencidos o sin existencias. Devuelve la asignación
// y cuántas unidades faltaron (0 si alcanza).
func planificarFEFO(lotes []loteFEFO, cantidad int64) ([]asignacionLote, int64) {
	vendibles := make([]loteFEFO, 0, len(lotes))
	for _, l := range lotes {
		if l.CantidadActual > 0 && l.DiasRestantes >= 0 {
			vendibles = append(vendibles, l)
		}
	}
	sort.SliceStable(vendibles, func(i, j int) bool {
		if vendibles[i].FechaVencimiento != vendibles[j].FechaVencimiento {
			return vendibles[i].FechaVencimiento < vendibles[j].FechaVencimiento
		}
		return vendibles[i].ID < vendibles[j].ID
	})

	plan := []asignacionLote{}
	pendiente := cantidad
	for _, l := range vendibles {
		if pendiente <= 0 {
			break
		}
		tomar := min(l.CantidadActual, pendiente)
		plan = append(plan, asignacionLote{
			LoteID:           l.ID,
			CodigoLote:       l.CodigoLote,
			FechaVencimiento: l.FechaVencimiento,
			DiasRestantes:    l.DiasRestantes,
			Disponible:       l.CantidadActual,
			Tomar:            tomar,
			Quedaria:         l.CantidadActual - tomar,
		})
		pendiente -= tomar
	}
	return plan, max(pendiente, 0)
}

var tiposEtiqueta = map[string]bool{
	"necesidad": true, "ingrediente": true, "contraindicacion": true, "otro": true,
}

// --- Modelos ---

type categoria struct {
	ID                int64  `json:"id"`
	Nombre            string `json:"nombre"`
	CantidadProductos int64  `json:"cantidad_productos"`
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
	StockVencido int64   `json:"stock_vencido"`
	StockMinimo  int64   `json:"stock_minimo"`
	StockBajo    bool    `json:"stock_bajo"`
}

type proveedor struct {
	ID                int64   `json:"id"`
	Nombre            string  `json:"nombre"`
	Telefono          *string `json:"telefono"`
	AceptaDevolucion  bool    `json:"acepta_devolucion"`
	CantidadProductos *int64  `json:"cantidad_productos,omitempty"`
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

type asignacionLote struct {
	LoteID           int64   `json:"lote_id"`
	CodigoLote       *string `json:"codigo_lote"`
	FechaVencimiento string  `json:"fecha_vencimiento"`
	DiasRestantes    int64   `json:"dias_restantes"`
	Disponible       int64   `json:"disponible"`
	Tomar            int64   `json:"tomar"`
	Quedaria         int64   `json:"quedaria"`
}

type alertaStock struct {
	productoResumen
	FaltanteParaMinimo int64 `json:"faltante_para_minimo"`
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
	CostoUnitario         float64 `json:"costo_unitario"`
	ValorEnRiesgo         float64 `json:"valor_en_riesgo"`
	DiasRestantes         int64   `json:"dias_restantes"`
	DiasAlertaLiquidacion int64   `json:"dias_alerta_liquidacion"`
	Nivel                 string  `json:"nivel"`
	AccionSugerida        string  `json:"accion_sugerida"`
	Proveedor             *string `json:"proveedor"`
	ProveedorTelefono     *string `json:"proveedor_telefono"`
	AceptaDevolucion      *bool   `json:"acepta_devolucion"`
}

// --- Consultas compartidas ---

// stockVigente: solo cuentan los lotes no vencidos.
const stockVigente = `COALESCE(SUM(CASE WHEN l.fecha_vencimiento >= CURRENT_DATE THEN l.cantidad_actual ELSE 0 END), 0)`

// stockVencido: unidades que siguen en lotes ya vencidos (deben darse de baja).
const stockVencido = `COALESCE(SUM(CASE WHEN l.fecha_vencimiento < CURRENT_DATE THEN l.cantidad_actual ELSE 0 END), 0)`

const selectProductoResumen = `
SELECT p.id, p.codigo_barras, p.nombre, p.marca, p.presentacion,
       c.id, c.nombre, p.precio_venta, p.es_granel, p.stock_minimo,
       ` + stockVigente + ` AS stock,
       ` + stockVencido + ` AS stock_vencido
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
			&p.CategoriaID, &p.Categoria, &p.PrecioVenta, &p.EsGranel, &p.StockMinimo,
			&p.Stock, &p.StockVencido); err != nil {
			return nil, err
		}
		p.StockBajo = p.Stock <= p.StockMinimo
		lista = append(lista, p)
	}
	return lista, rows.Err()
}

func (a *app) lotesFEFO(ctx context.Context, productoID int64) ([]loteFEFO, error) {
	rows, err := a.db.QueryContext(ctx, `
SELECT id, codigo_lote, fecha_vencimiento, cantidad_actual,
       DATEDIFF(fecha_vencimiento, CURRENT_DATE) AS dias_restantes
FROM lotes
WHERE producto_id = ? AND cantidad_actual > 0 AND fecha_vencimiento >= CURRENT_DATE
ORDER BY fecha_vencimiento, id`, productoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lotes := []loteFEFO{}
	for rows.Next() {
		var l loteFEFO
		var venc time.Time
		if err := rows.Scan(&l.ID, &l.CodigoLote, &venc, &l.CantidadActual, &l.DiasRestantes); err != nil {
			return nil, err
		}
		l.FechaVencimiento = fechaTexto(venc)
		lotes = append(lotes, l)
	}
	return lotes, rows.Err()
}

func (a *app) etiquetasPorIDs(ctx context.Context, ids []int64) ([]etiqueta, error) {
	lista := []etiqueta{}
	if len(ids) == 0 {
		return lista, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := a.db.QueryContext(ctx,
		`SELECT id, nombre, tipo FROM etiquetas WHERE activa = 1 AND id IN (`+placeholders(len(ids))+`) ORDER BY nombre`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var e etiqueta
		if err := rows.Scan(&e.ID, &e.Nombre, &e.Tipo); err != nil {
			return nil, err
		}
		lista = append(lista, e)
	}
	return lista, rows.Err()
}

// --- Handlers: salud ---

// GET /api/salud
func (a *app) salud(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	inicio := time.Now()
	var version, base string
	if err := a.db.QueryRowContext(ctx, `SELECT VERSION(), DATABASE()`).Scan(&version, &base); err != nil {
		log.Printf("salud: MySQL no responde: %v", err)
		escribirJSON(w, http.StatusInternalServerError, map[string]string{
			"api": "ok", "error": "sin conexión con MySQL",
		})
		return
	}
	escribirJSON(w, http.StatusOK, map[string]any{
		"api":           "ok",
		"mysql":         version,
		"base":          base,
		"latencia_ms":   time.Since(inicio).Milliseconds(),
		"hora_servidor": time.Now().Format(time.RFC3339),
	})
}

// --- Handlers: categorías ---

// GET /api/categorias
func (a *app) listarCategorias(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	rows, err := a.db.QueryContext(ctx, `
SELECT c.id, c.nombre, COUNT(p.id)
FROM categorias c
LEFT JOIN productos p ON p.categoria_id = c.id AND p.activo = 1
GROUP BY c.id, c.nombre
ORDER BY c.nombre`)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	defer rows.Close()
	lista := []categoria{}
	for rows.Next() {
		var c categoria
		if err := rows.Scan(&c.ID, &c.Nombre, &c.CantidadProductos); err != nil {
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

// GET /api/categorias/{id}
func (a *app) obtenerCategoria(w http.ResponseWriter, r *http.Request) {
	id, ok := idDeRuta(r)
	if !ok {
		escribirError(w, http.StatusBadRequest, "el id debe ser un número entero positivo")
		return
	}
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	var c categoria
	err := a.db.QueryRowContext(ctx, `
SELECT c.id, c.nombre, COUNT(p.id)
FROM categorias c
LEFT JOIN productos p ON p.categoria_id = c.id AND p.activo = 1
WHERE c.id = ?
GROUP BY c.id, c.nombre`, id).Scan(&c.ID, &c.Nombre, &c.CantidadProductos)
	if errors.Is(err, sql.ErrNoRows) {
		escribirError(w, http.StatusNotFound, "categoría no encontrada")
		return
	}
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	escribirJSON(w, http.StatusOK, c)
}

// POST /api/categorias  {"nombre":"..."}
func (a *app) crearCategoria(w http.ResponseWriter, r *http.Request) {
	var cuerpo struct {
		Nombre *string `json:"nombre"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCuerpo))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cuerpo); err != nil {
		var maxErr *http.MaxBytesError
		var tipoErr *json.UnmarshalTypeError
		msg := "JSON inválido; se espera {\"nombre\":\"...\"}"
		switch {
		case errors.Is(err, io.EOF):
			msg = "el cuerpo está vacío; se espera {\"nombre\":\"...\"}"
		case errors.As(err, &maxErr):
			msg = "el cuerpo supera el tamaño máximo (1 MB)"
		case errors.As(err, &tipoErr):
			msg = "el campo \"" + tipoErr.Field + "\" debe ser texto"
		case strings.HasPrefix(err.Error(), "json: unknown field"):
			msg = "campo no permitido: " + strings.TrimPrefix(err.Error(), "json: unknown field ") + "; solo se acepta \"nombre\""
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

// DELETE /api/categorias/{id}
// Solo se puede eliminar una categoría sin productos (ni activos ni inactivos);
// así las categorías con historial quedan protegidas.
func (a *app) eliminarCategoria(w http.ResponseWriter, r *http.Request) {
	id, ok := idDeRuta(r)
	if !ok {
		escribirError(w, http.StatusBadRequest, "el id debe ser un número entero positivo")
		return
	}
	ctx, cancel := ctxConsulta(r)
	defer cancel()

	var existe, enUso int64
	err := a.db.QueryRowContext(ctx, `
SELECT (SELECT COUNT(*) FROM categorias WHERE id = ?),
       (SELECT COUNT(*) FROM productos WHERE categoria_id = ?)`, id, id).Scan(&existe, &enUso)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	if existe == 0 {
		escribirError(w, http.StatusNotFound, "categoría no encontrada")
		return
	}
	if enUso > 0 {
		escribirError(w, http.StatusConflict, fmt.Sprintf("no se puede eliminar: la categoría tiene %d producto(s) asociado(s)", enUso))
		return
	}

	res, err := a.db.ExecContext(ctx, `DELETE FROM categorias WHERE id = ?`, id)
	if err != nil {
		var me *mysql.MySQLError
		if errors.As(err, &me) && me.Number == errMySQLFKReferencia {
			escribirError(w, http.StatusConflict, "no se puede eliminar: la categoría tiene productos asociados")
			return
		}
		errorInterno(w, r, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		escribirError(w, http.StatusNotFound, "categoría no encontrada")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Handlers: proveedores ---

// GET /api/proveedores
func (a *app) listarProveedores(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	rows, err := a.db.QueryContext(ctx, `
SELECT pr.id, pr.nombre, pr.telefono, pr.acepta_devolucion, COUNT(p.id)
FROM proveedores pr
LEFT JOIN productos p ON p.proveedor_id = pr.id AND p.activo = 1
GROUP BY pr.id, pr.nombre, pr.telefono, pr.acepta_devolucion
ORDER BY pr.nombre`)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	defer rows.Close()
	lista := []proveedor{}
	for rows.Next() {
		var p proveedor
		var n int64
		if err := rows.Scan(&p.ID, &p.Nombre, &p.Telefono, &p.AceptaDevolucion, &n); err != nil {
			errorInterno(w, r, err)
			return
		}
		p.CantidadProductos = &n
		lista = append(lista, p)
	}
	if err := rows.Err(); err != nil {
		errorInterno(w, r, err)
		return
	}
	escribirJSON(w, http.StatusOK, lista)
}

// --- Handlers: productos ---

// GET /api/productos?q=texto
func (a *app) listarProductos(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if utf8.RuneCountInString(q) > maxBusqueda {
		escribirError(w, http.StatusBadRequest, "la búsqueda no puede superar 100 caracteres")
		return
	}
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
       `+stockVencido+` AS stock_vencido,
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
		&f.Stock, &f.StockVencido,
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

	lotes, err := a.lotesFEFO(ctx, id)
	if err != nil {
		errorInterno(w, r, err)
		return
	}

	escribirJSON(w, http.StatusOK, map[string]any{
		"ficha":     f,
		"etiquetas": etiquetas,
		"lotes":     lotes,
	})
}

// GET /api/productos/{id}/plan-fefo?cantidad=N
// Simula de qué lotes saldría una venta de N unidades (no modifica el inventario).
func (a *app) planFEFO(w http.ResponseWriter, r *http.Request) {
	id, ok := idDeRuta(r)
	if !ok {
		escribirError(w, http.StatusBadRequest, "el id debe ser un número entero positivo")
		return
	}
	cantidadTxt := r.URL.Query().Get("cantidad")
	cantidad, ok := parsearIDPositivo(cantidadTxt)
	if cantidadTxt == "" {
		escribirError(w, http.StatusBadRequest, "falta el parámetro cantidad (entero mayor que 0)")
		return
	}
	if !ok || cantidad > maxCantidadFEFO {
		escribirError(w, http.StatusBadRequest, "cantidad debe ser un entero entre 1 y 1000000")
		return
	}

	ctx, cancel := ctxConsulta(r)
	defer cancel()
	var nombre string
	err := a.db.QueryRowContext(ctx, `SELECT nombre FROM productos WHERE id = ? AND activo = 1`, id).Scan(&nombre)
	if errors.Is(err, sql.ErrNoRows) {
		escribirError(w, http.StatusNotFound, "producto no encontrado")
		return
	}
	if err != nil {
		errorInterno(w, r, err)
		return
	}

	lotes, err := a.lotesFEFO(ctx, id)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	var disponible int64
	for _, l := range lotes {
		disponible += l.CantidadActual
	}
	plan, faltante := planificarFEFO(lotes, cantidad)
	if faltante > 0 {
		escribirError(w, http.StatusConflict, fmt.Sprintf(
			"stock insuficiente: se pidieron %d y hay %d disponibles en lotes vigentes", cantidad, disponible))
		return
	}
	escribirJSON(w, http.StatusOK, map[string]any{
		"producto_id":         id,
		"producto":            nombre,
		"cantidad_solicitada": cantidad,
		"stock_vigente":       disponible,
		"lotes_usados":        len(plan),
		"asignacion":          plan,
		"nota":                "Simulación: no modifica el inventario.",
	})
}

// --- Handlers: etiquetas y recomendaciones ---

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

// GET /api/recomendaciones?necesidad=ID&evitar=ID,ID&disponibles=1
// Productos para una necesidad, excluyendo los que tengan alguna de las
// etiquetas indicadas en "evitar" (por ejemplo, embarazo o hipertensión).
func (a *app) recomendaciones(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	necesidadTxt := q.Get("necesidad")
	if strings.TrimSpace(necesidadTxt) == "" {
		escribirError(w, http.StatusBadRequest, "falta el parámetro necesidad (id de una etiqueta de tipo necesidad)")
		return
	}
	necesidadID, ok := parsearIDPositivo(necesidadTxt)
	if !ok {
		escribirError(w, http.StatusBadRequest, "necesidad debe ser un número entero positivo")
		return
	}
	evitarIDs, err := parsearListaIDs(q.Get("evitar"), maxEtiquetasEvitar)
	if err != nil {
		escribirError(w, http.StatusBadRequest, "evitar debe ser una lista de ids separados por coma: "+err.Error())
		return
	}
	soloDisponibles := false
	switch strings.ToLower(strings.TrimSpace(q.Get("disponibles"))) {
	case "", "0", "false", "no":
	case "1", "true", "si", "sí":
		soloDisponibles = true
	default:
		escribirError(w, http.StatusBadRequest, "disponibles debe ser 1 o 0")
		return
	}

	ctx, cancel := ctxConsulta(r)
	defer cancel()

	var nec etiqueta
	err = a.db.QueryRowContext(ctx,
		`SELECT id, nombre, tipo FROM etiquetas WHERE id = ? AND activa = 1`, necesidadID).Scan(&nec.ID, &nec.Nombre, &nec.Tipo)
	if errors.Is(err, sql.ErrNoRows) {
		escribirError(w, http.StatusNotFound, "etiqueta de necesidad no encontrada")
		return
	}
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	if nec.Tipo != "necesidad" {
		escribirError(w, http.StatusBadRequest, "la etiqueta "+strconv.FormatInt(nec.ID, 10)+" es de tipo "+nec.Tipo+", no de necesidad")
		return
	}

	evitando, err := a.etiquetasPorIDs(ctx, evitarIDs)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	if len(evitando) != len(evitarIDs) {
		escribirError(w, http.StatusNotFound, "alguna de las etiquetas en evitar no existe o está inactiva")
		return
	}

	consulta := selectProductoResumen + `
JOIN producto_etiqueta pe ON pe.producto_id = p.id
WHERE p.activo = 1 AND pe.etiqueta_id = ?`
	args := []any{necesidadID}
	if len(evitarIDs) > 0 {
		consulta += `
  AND NOT EXISTS (SELECT 1 FROM producto_etiqueta px
                  WHERE px.producto_id = p.id AND px.etiqueta_id IN (` + placeholders(len(evitarIDs)) + `))`
		for _, id := range evitarIDs {
			args = append(args, id)
		}
	}
	consulta += agruparProductoResumen
	if soloDisponibles {
		consulta += ` HAVING stock > 0 `
	}
	consulta += `ORDER BY (stock > 0) DESC, p.nombre, p.id`

	rows, err := a.db.QueryContext(ctx, consulta, args...)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	lista, err := escanearProductos(rows)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	escribirJSON(w, http.StatusOK, map[string]any{
		"necesidad": nec,
		"evitando":  evitando,
		"productos": lista,
		"aviso":     avisoOrientativo,
	})
}

// --- Handlers: alertas ---

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
	productos, err := escanearProductos(rows)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	lista := make([]alertaStock, 0, len(productos))
	for _, p := range productos {
		lista = append(lista, alertaStock{productoResumen: p, FaltanteParaMinimo: max(p.StockMinimo-p.Stock, 0)})
	}
	escribirJSON(w, http.StatusOK, lista)
}

// GET /api/alertas/vencimientos
func (a *app) alertasVencimientos(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	rows, err := a.db.QueryContext(ctx, `
SELECT l.id, l.codigo_lote, p.id, p.nombre, p.marca, p.presentacion,
       l.fecha_vencimiento, l.cantidad_actual, l.costo_unitario,
       l.cantidad_actual * l.costo_unitario AS valor_en_riesgo,
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
			&venc, &al.CantidadActual, &al.CostoUnitario, &al.ValorEnRiesgo, &al.DiasRestantes, &al.DiasAlertaLiquidacion,
			&al.Proveedor, &al.ProveedorTelefono, &al.AceptaDevolucion); err != nil {
			errorInterno(w, r, err)
			return
		}
		al.FechaVencimiento = fechaTexto(venc)
		al.Nivel = clasificarVencimiento(int(al.DiasRestantes), int(al.DiasAlertaLiquidacion))
		al.AccionSugerida = sugerirAccion(al.Nivel, al.AceptaDevolucion != nil && *al.AceptaDevolucion)
		lista = append(lista, al)
	}
	if err := rows.Err(); err != nil {
		errorInterno(w, r, err)
		return
	}
	escribirJSON(w, http.StatusOK, lista)
}
