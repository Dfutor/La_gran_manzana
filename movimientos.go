package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// dinero representa centavos exactos: los totales nunca se calculan con float64.
type dinero int64

const maxDinero dinero = 999999999999 // DECIMAL(12,2)
func parsearDinero(s string) (dinero, error) {
	if s == "" || strings.HasPrefix(s, "-") || strings.HasPrefix(s, "+") {
		return 0, fmt.Errorf("importe inválido")
	}
	partes := strings.Split(s, ".")
	if len(partes) > 2 || partes[0] == "" {
		return 0, fmt.Errorf("importe inválido")
	}
	for _, c := range s {
		if (c < '0' || c > '9') && c != '.' {
			return 0, fmt.Errorf("importe inválido")
		}
	}
	dec := "00"
	if len(partes) == 2 {
		if len(partes[1]) == 0 || len(partes[1]) > 2 {
			return 0, fmt.Errorf("use hasta dos decimales")
		}
		dec = (partes[1] + "0")[:2]
	}
	entero, err := strconv.ParseInt(partes[0], 10, 64)
	if err != nil || entero > 9999999999 {
		return 0, fmt.Errorf("importe fuera de rango")
	}
	cent, _ := strconv.ParseInt(dec, 10, 64)
	return dinero(entero*100 + cent), nil
}
func (d dinero) String() string               { return fmt.Sprintf("%d.%02d", int64(d)/100, int64(d)%100) }
func (d dinero) MarshalJSON() ([]byte, error) { return []byte(d.String()), nil }
func (d *dinero) UnmarshalJSON(b []byte) error {
	var s string
	if len(b) > 0 && b[0] == '"' {
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
	} else {
		s = string(b)
	}
	v, err := parsearDinero(s)
	if err == nil {
		*d = v
	}
	return err
}
func (d *dinero) Scan(v any) error {
	var s string
	switch v := v.(type) {
	case []byte:
		s = string(v)
	case string:
		s = v
	case int64:
		s = strconv.FormatInt(v, 10)
	default:
		return fmt.Errorf("importe SQL inválido")
	}
	n, err := parsearDinero(s)
	if err == nil {
		*d = n
	}
	return err
}

type lineaVentaEntrada struct {
	ProductoID     int64   `json:"producto_id"`
	Cantidad       int64   `json:"cantidad"`
	PrecioUnitario *dinero `json:"precio_unitario,omitempty"`
}
type ventaEntrada struct {
	MedioPago string              `json:"medio_pago"`
	Detalles  []lineaVentaEntrada `json:"detalles"`
}

func validarVenta(in *ventaEntrada) error {
	if in.MedioPago != "efectivo" && in.MedioPago != "nequi" && in.MedioPago != "daviplata" {
		return fmt.Errorf("medio_pago debe ser efectivo, nequi o daviplata")
	}
	if len(in.Detalles) == 0 || len(in.Detalles) > 100 {
		return fmt.Errorf("incluya entre 1 y 100 productos")
	}
	vistos := map[int64]bool{}
	for _, l := range in.Detalles {
		if l.ProductoID <= 0 || l.Cantidad < 1 || l.Cantidad > maxCantidadFEFO || vistos[l.ProductoID] {
			return fmt.Errorf("productos únicos e ids positivos; cantidad entre 1 y %d", maxCantidadFEFO)
		}
		vistos[l.ProductoID] = true
	}
	sort.Slice(in.Detalles, func(i, j int) bool { return in.Detalles[i].ProductoID < in.Detalles[j].ProductoID })
	return nil
}

type lineaVenta struct {
	ProductoID     int64            `json:"producto_id"`
	Cantidad       int64            `json:"cantidad"`
	PrecioUnitario dinero           `json:"precio_unitario"`
	Subtotal       dinero           `json:"subtotal"`
	Asignacion     []asignacionLote `json:"asignacion"`
}

func (a *app) crearVenta(w http.ResponseWriter, r *http.Request) {
	var in ventaEntrada
	if !decodificarJSON(w, r, &in) {
		return
	}
	if err := validarVenta(&in); err != nil {
		escribirError(w, 400, err.Error())
		return
	}
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	tx, err := a.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	defer tx.Rollback()
	detalles := []lineaVenta{}
	var total dinero
	// Bloqueo de productos en orden estable: evita doble consumo y reduce interbloqueos.
	for _, entrada := range in.Detalles {
		var precio dinero
		var activo bool
		err = tx.QueryRowContext(ctx, `SELECT precio_venta,activo FROM productos WHERE id=? FOR UPDATE`, entrada.ProductoID).Scan(&precio, &activo)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && !activo) {
			escribirError(w, 404, "producto no encontrado o inactivo")
			return
		}
		if err != nil {
			errorInterno(w, r, err)
			return
		}
		if entrada.PrecioUnitario != nil {
			precio = *entrada.PrecioUnitario
		}
		if precio > maxDinero/dinero(entrada.Cantidad) {
			escribirError(w, 400, "total de venta fuera de rango")
			return
		}
		subtotal := precio * dinero(entrada.Cantidad)
		if total > maxDinero-subtotal {
			escribirError(w, 400, "total de venta fuera de rango")
			return
		}
		total += subtotal
		rows, err := tx.QueryContext(ctx, `SELECT id,codigo_lote,fecha_vencimiento,cantidad_actual,DATEDIFF(fecha_vencimiento,CURRENT_DATE) FROM lotes WHERE producto_id=? AND cantidad_actual>0 AND fecha_vencimiento>=CURRENT_DATE ORDER BY fecha_vencimiento,id FOR UPDATE`, entrada.ProductoID)
		if err != nil {
			errorInterno(w, r, err)
			return
		}
		lotes := []loteFEFO{}
		for rows.Next() {
			var l loteFEFO
			var f time.Time
			if err = rows.Scan(&l.ID, &l.CodigoLote, &f, &l.CantidadActual, &l.DiasRestantes); err != nil {
				break
			}
			l.FechaVencimiento = fechaTexto(f)
			lotes = append(lotes, l)
		}
		iterErr := rows.Err()
		rows.Close()
		if err != nil {
			errorInterno(w, r, err)
			return
		}
		if iterErr != nil {
			errorInterno(w, r, iterErr)
			return
		}
		plan, faltante := planificarFEFO(lotes, entrada.Cantidad)
		if faltante > 0 {
			escribirError(w, 409, fmt.Sprintf("stock insuficiente del producto %d; faltan %d unidades", entrada.ProductoID, faltante))
			return
		}
		for _, paso := range plan {
			if _, err = tx.ExecContext(ctx, `UPDATE lotes SET cantidad_actual=cantidad_actual-? WHERE id=?`, paso.Tomar, paso.LoteID); err != nil {
				errorInterno(w, r, err)
				return
			}
		}
		detalles = append(detalles, lineaVenta{entrada.ProductoID, entrada.Cantidad, precio, subtotal, plan})
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO ventas(usuario_id,medio_pago,total) VALUES(?,?,?)`, usuarioActual(r).ID, in.MedioPago, total.String())
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	id, err := res.LastInsertId()
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	for _, l := range detalles {
		if _, err = tx.ExecContext(ctx, `INSERT INTO detalle_venta(venta_id,producto_id,cantidad,precio_unitario) VALUES(?,?,?,?)`, id, l.ProductoID, l.Cantidad, l.PrecioUnitario.String()); err != nil {
			errorInterno(w, r, err)
			return
		}
	}
	if err = tx.Commit(); err != nil {
		errorInterno(w, r, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/api/ventas/%d", id))
	escribirJSON(w, 201, map[string]any{"id": id, "usuario_id": usuarioActual(r).ID, "medio_pago": in.MedioPago, "total": total, "detalles": detalles})
}

type loteEntrada struct {
	ProductoID       int64   `json:"producto_id"`
	CodigoLote       *string `json:"codigo_lote"`
	NumeroFactura    *string `json:"numero_factura"`
	FechaVencimiento string  `json:"fecha_vencimiento"`
	Cantidad         int64   `json:"cantidad"`
	CostoUnitario    *dinero `json:"costo_unitario"`
}

func validarLote(in loteEntrada) error {
	if in.ProductoID <= 0 || in.Cantidad < 1 || in.Cantidad > maxCantidadFEFO || in.CostoUnitario == nil {
		return fmt.Errorf("producto_id, cantidad (1 a %d) y costo_unitario son obligatorios", maxCantidadFEFO)
	}
	if _, err := time.Parse(formatoFecha, in.FechaVencimiento); err != nil {
		return fmt.Errorf("fecha_vencimiento debe ser AAAA-MM-DD")
	}
	if (in.CodigoLote != nil && utf8.RuneCountInString(*in.CodigoLote) > 60) || (in.NumeroFactura != nil && utf8.RuneCountInString(*in.NumeroFactura) > 50) {
		return fmt.Errorf("código de lote o factura demasiado largo")
	}
	return nil
}
func (a *app) crearLote(w http.ResponseWriter, r *http.Request) {
	var in loteEntrada
	if !decodificarJSON(w, r, &in) {
		return
	}
	if err := validarLote(in); err != nil {
		escribirError(w, 400, err.Error())
		return
	}
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	// La fecha de MySQL es la misma referencia de FEFO y de alertas.
	var valido bool
	if err := a.db.QueryRowContext(ctx, `SELECT ?>=CURRENT_DATE`, in.FechaVencimiento).Scan(&valido); err != nil {
		errorInterno(w, r, err)
		return
	}
	if !valido {
		escribirError(w, 400, "no se puede ingresar un lote vencido")
		return
	}
	res, err := a.db.ExecContext(ctx, `INSERT INTO lotes(producto_id,codigo_lote,numero_factura,fecha_vencimiento,cantidad_inicial,cantidad_actual,costo_unitario) SELECT id,?,?,?,?,?,? FROM productos WHERE id=? AND activo=1`, in.CodigoLote, in.NumeroFactura, in.FechaVencimiento, in.Cantidad, in.Cantidad, in.CostoUnitario.String(), in.ProductoID)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	n, err := res.RowsAffected()
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	if n == 0 {
		escribirError(w, 404, "producto no encontrado o inactivo")
		return
	}
	id, err := res.LastInsertId()
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	escribirJSON(w, 201, map[string]any{"id": id, "cantidad_actual": in.Cantidad})
}
func (a *app) actualizarLote(w http.ResponseWriter, r *http.Request) {
	id, ok := idDeRuta(r)
	if !ok {
		escribirError(w, 400, "id inválido")
		return
	}
	// Las cantidades solo cambian con ingresos, ventas y bajas.
	var in struct {
		CodigoLote       *string `json:"codigo_lote"`
		NumeroFactura    *string `json:"numero_factura"`
		FechaVencimiento string  `json:"fecha_vencimiento"`
	}
	if !decodificarJSON(w, r, &in) {
		return
	}
	cero := dinero(0)
	if err := validarLote(loteEntrada{1, in.CodigoLote, in.NumeroFactura, in.FechaVencimiento, 1, &cero}); err != nil {
		escribirError(w, 400, err.Error())
		return
	}
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	var n int
	if err := a.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM lotes WHERE id=?`, id).Scan(&n); err != nil {
		errorInterno(w, r, err)
		return
	}
	if n == 0 {
		escribirError(w, 404, "lote no encontrado")
		return
	}
	if _, err := a.db.ExecContext(ctx, `UPDATE lotes SET codigo_lote=?,numero_factura=?,fecha_vencimiento=? WHERE id=?`, in.CodigoLote, in.NumeroFactura, in.FechaVencimiento, id); err != nil {
		errorInterno(w, r, err)
		return
	}
	escribirJSON(w, 200, map[string]any{"id": id})
}

var motivosBaja = map[string]bool{"vencimiento": true, "deterioro": true, "obsequio": true, "devolucion": true, "otro": true}
var accionesBaja = map[string]bool{"ninguna": true, "venta_destacada": true, "rebaja": true, "combo": true, "devolucion_distribuidor": true, "obsequio": true}

type bajaEntrada struct {
	LoteID       int64  `json:"lote_id"`
	Cantidad     int64  `json:"cantidad"`
	Motivo       string `json:"motivo"`
	AccionPrevia string `json:"accion_previa"`
}

func validarBaja(in *bajaEntrada) error {
	if in.AccionPrevia == "" {
		in.AccionPrevia = "ninguna"
	}
	if in.LoteID <= 0 || in.Cantidad < 1 || in.Cantidad > maxCantidadFEFO || !motivosBaja[in.Motivo] || !accionesBaja[in.AccionPrevia] {
		return fmt.Errorf("lote, cantidad, motivo o accion_previa inválidos")
	}
	return nil
}
func (a *app) crearBaja(w http.ResponseWriter, r *http.Request) {
	var in bajaEntrada
	if !decodificarJSON(w, r, &in) {
		return
	}
	if err := validarBaja(&in); err != nil {
		escribirError(w, 400, err.Error())
		return
	}
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	tx, err := a.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	defer tx.Rollback()
	// Mismo orden de bloqueo que ventas: producto y luego lote.
	var productoID int64
	err = tx.QueryRowContext(ctx, `SELECT producto_id FROM lotes WHERE id=?`, in.LoteID).Scan(&productoID)
	if errors.Is(err, sql.ErrNoRows) {
		escribirError(w, 404, "lote no encontrado")
		return
	}
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	var bloqueado int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM productos WHERE id=? FOR UPDATE`, productoID).Scan(&bloqueado); err != nil {
		errorInterno(w, r, err)
		return
	}
	var stock int64
	var costo dinero
	var vencido bool
	if err = tx.QueryRowContext(ctx, `SELECT cantidad_actual,costo_unitario,fecha_vencimiento<CURRENT_DATE FROM lotes WHERE id=? FOR UPDATE`, in.LoteID).Scan(&stock, &costo, &vencido); err != nil {
		errorInterno(w, r, err)
		return
	}
	if costo > dinero(99999999999999)/dinero(in.Cantidad) {
		escribirError(w, 400, "valor de la baja fuera de rango")
		return
	}
	if stock < in.Cantidad {
		escribirError(w, 409, "cantidad superior al stock del lote")
		return
	}
	if in.Motivo == "vencimiento" && !vencido {
		escribirError(w, 400, "el lote aún no está vencido")
		return
	}
	if _, err = tx.ExecContext(ctx, `UPDATE lotes SET cantidad_actual=cantidad_actual-? WHERE id=?`, in.Cantidad, in.LoteID); err != nil {
		errorInterno(w, r, err)
		return
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO bajas(producto_id,lote_id,cantidad,motivo,accion_previa,costo_unitario,usuario_id) VALUES(?,?,?,?,?,?,?)`, productoID, in.LoteID, in.Cantidad, in.Motivo, in.AccionPrevia, costo.String(), usuarioActual(r).ID)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	id, err := res.LastInsertId()
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	if err = tx.Commit(); err != nil {
		errorInterno(w, r, err)
		return
	}
	escribirJSON(w, 201, map[string]any{"id": id, "valor_perdido": costo * dinero(in.Cantidad), "cantidad_actual": stock - in.Cantidad})
}
