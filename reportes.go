package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
)

// Solo se llama con sentencias constantes del servidor y parámetros separados.
func responderFilas(w http.ResponseWriter, r *http.Request, rows *sql.Rows, err error) {
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	defer rows.Close()
	columnas, err := rows.Columns()
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	tipos, err := rows.ColumnTypes()
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	lista := []map[string]any{}
	for rows.Next() {
		valores := make([]any, len(columnas))
		punteros := make([]any, len(columnas))
		for i := range valores {
			punteros[i] = &valores[i]
		}
		if err = rows.Scan(punteros...); err != nil {
			errorInterno(w, r, err)
			return
		}
		fila := map[string]any{}
		for i, v := range valores {
			switch v := v.(type) {
			case []byte:
				s := string(v)
				switch tipos[i].DatabaseTypeName() {
				case "DECIMAL", "NEWDECIMAL", "BIGINT", "INT", "SMALLINT", "TINYINT":
					fila[columnas[i]] = json.Number(s)
				default:
					fila[columnas[i]] = s
				}
			case time.Time:
				if tipos[i].DatabaseTypeName() == "DATE" {
					fila[columnas[i]] = fechaTexto(v)
				} else {
					fila[columnas[i]] = v.Format("2006-01-02T15:04:05")
				}
			default:
				fila[columnas[i]] = v
			}
		}
		lista = append(lista, fila)
	}
	if err = rows.Err(); err != nil {
		errorInterno(w, r, err)
		return
	}
	escribirJSON(w, 200, lista)
}
func paginacion(r *http.Request) (limite, offset int, err error) {
	limite = 100
	if s := r.URL.Query().Get("limite"); s != "" {
		limite, err = strconv.Atoi(s)
		if err != nil || limite < 1 || limite > 500 {
			return 0, 0, fmt.Errorf("limite debe estar entre 1 y 500")
		}
	}
	if s := r.URL.Query().Get("offset"); s != "" {
		offset, err = strconv.Atoi(s)
		if err != nil || offset < 0 {
			return 0, 0, fmt.Errorf("offset inválido")
		}
	}
	return
}
func periodo(r *http.Request) (desde, hasta string, err error) {
	loc, e := time.LoadLocation("America/Bogota")
	if e != nil {
		return "", "", e
	}
	hoy := time.Now().In(loc)
	desde = hoy.AddDate(0, 0, -29).Format(formatoFecha)
	hasta = hoy.Format(formatoFecha)
	if s := r.URL.Query().Get("desde"); s != "" {
		desde = s
	}
	if s := r.URL.Query().Get("hasta"); s != "" {
		hasta = s
	}
	d, e := time.Parse(formatoFecha, desde)
	if e != nil {
		return "", "", fmt.Errorf("desde debe ser AAAA-MM-DD")
	}
	h, e := time.Parse(formatoFecha, hasta)
	if e != nil {
		return "", "", fmt.Errorf("hasta debe ser AAAA-MM-DD")
	}
	if h.Before(d) || h.Sub(d) > 366*24*time.Hour {
		return "", "", fmt.Errorf("el periodo debe ser de 1 a 367 días")
	}
	return
}
func (a *app) listarLotes(w http.ResponseWriter, r *http.Request) {
	limite, offset, err := paginacion(r)
	if err != nil {
		escribirError(w, 400, err.Error())
		return
	}
	var id int64
	if s := r.URL.Query().Get("producto_id"); s != "" {
		var ok bool
		id, ok = parsearIDPositivo(s)
		if !ok {
			escribirError(w, 400, "producto_id inválido")
			return
		}
	}
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	rows, err := a.db.QueryContext(ctx, `SELECT l.id,l.producto_id,p.nombre AS producto,l.codigo_lote,l.numero_factura,l.fecha_vencimiento,l.cantidad_inicial,l.cantidad_actual,l.costo_unitario,l.fecha_ingreso,DATEDIFF(l.fecha_vencimiento,CURRENT_DATE) AS dias_restantes FROM lotes l JOIN productos p ON p.id=l.producto_id WHERE (?=0 OR l.producto_id=?) ORDER BY l.fecha_vencimiento,l.id LIMIT ? OFFSET ?`, id, id, limite, offset)
	responderFilas(w, r, rows, err)
}
func (a *app) listarVentas(w http.ResponseWriter, r *http.Request) {
	desde, hasta, err := periodo(r)
	if err != nil {
		escribirError(w, 400, err.Error())
		return
	}
	limite, offset, err := paginacion(r)
	if err != nil {
		escribirError(w, 400, err.Error())
		return
	}
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	rows, err := a.db.QueryContext(ctx, `SELECT v.id,v.usuario_id,u.nombre AS usuario,v.fecha_hora,v.medio_pago,v.total FROM ventas v JOIN usuarios u ON u.id=v.usuario_id WHERE v.fecha_hora>=? AND v.fecha_hora<DATE_ADD(?,INTERVAL 1 DAY) ORDER BY v.fecha_hora DESC,v.id DESC LIMIT ? OFFSET ?`, desde, hasta, limite, offset)
	responderFilas(w, r, rows, err)
}
func (a *app) detalleVenta(w http.ResponseWriter, r *http.Request) {
	id, ok := idDeRuta(r)
	if !ok {
		escribirError(w, 400, "id inválido")
		return
	}
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	var u int64
	var fecha time.Time
	var pago string
	var total dinero
	err := a.db.QueryRowContext(ctx, `SELECT usuario_id,fecha_hora,medio_pago,total FROM ventas WHERE id=?`, id).Scan(&u, &fecha, &pago, &total)
	if err == sql.ErrNoRows {
		escribirError(w, 404, "venta no encontrada")
		return
	}
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	rows, err := a.db.QueryContext(ctx, `SELECT d.producto_id,p.nombre,d.cantidad,d.precio_unitario,d.subtotal FROM detalle_venta d JOIN productos p ON p.id=d.producto_id WHERE d.venta_id=? ORDER BY d.id`, id)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	defer rows.Close()
	type detalle struct {
		ProductoID int64  `json:"producto_id"`
		Nombre     string `json:"nombre"`
		Cantidad   int64  `json:"cantidad"`
		Precio     dinero `json:"precio_unitario"`
		Subtotal   dinero `json:"subtotal"`
	}
	detalles := []detalle{}
	for rows.Next() {
		var d detalle
		if err = rows.Scan(&d.ProductoID, &d.Nombre, &d.Cantidad, &d.Precio, &d.Subtotal); err != nil {
			errorInterno(w, r, err)
			return
		}
		detalles = append(detalles, d)
	}
	if err = rows.Err(); err != nil {
		errorInterno(w, r, err)
		return
	}
	escribirJSON(w, 200, map[string]any{"id": id, "usuario_id": u, "fecha_hora": fecha.Format("2006-01-02T15:04:05"), "medio_pago": pago, "total": total, "detalles": detalles})
}
func (a *app) listarBajas(w http.ResponseWriter, r *http.Request) {
	desde, hasta, err := periodo(r)
	if err != nil {
		escribirError(w, 400, err.Error())
		return
	}
	limite, offset, err := paginacion(r)
	if err != nil {
		escribirError(w, 400, err.Error())
		return
	}
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	rows, err := a.db.QueryContext(ctx, `SELECT b.id,b.producto_id,p.nombre AS producto,b.lote_id,b.cantidad,b.motivo,b.accion_previa,b.costo_unitario,b.valor_perdido,b.fecha_baja,b.usuario_id FROM bajas b JOIN productos p ON p.id=b.producto_id WHERE b.fecha_baja>=? AND b.fecha_baja<DATE_ADD(?,INTERVAL 1 DAY) ORDER BY b.fecha_baja DESC,b.id DESC LIMIT ? OFFSET ?`, desde, hasta, limite, offset)
	responderFilas(w, r, rows, err)
}
func (a *app) reporteVentas(w http.ResponseWriter, r *http.Request) {
	desde, hasta, err := periodo(r)
	if err != nil {
		escribirError(w, 400, err.Error())
		return
	}
	grupo := r.URL.Query().Get("agrupar")
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	var rows *sql.Rows
	switch grupo {
	case "", "dia":
		rows, err = a.db.QueryContext(ctx, `SELECT DATE(fecha_hora) AS dia,medio_pago,COUNT(*) AS ventas,SUM(total) AS total FROM ventas WHERE fecha_hora>=? AND fecha_hora<DATE_ADD(?,INTERVAL 1 DAY) GROUP BY DATE(fecha_hora),medio_pago ORDER BY dia,medio_pago`, desde, hasta)
	case "producto":
		rows, err = a.db.QueryContext(ctx, `SELECT p.id AS producto_id,p.nombre,SUM(d.cantidad) AS unidades,SUM(d.subtotal) AS total FROM detalle_venta d JOIN ventas v ON v.id=d.venta_id JOIN productos p ON p.id=d.producto_id WHERE v.fecha_hora>=? AND v.fecha_hora<DATE_ADD(?,INTERVAL 1 DAY) GROUP BY p.id,p.nombre ORDER BY unidades DESC,p.id`, desde, hasta)
	default:
		escribirError(w, 400, "agrupar debe ser dia o producto")
		return
	}
	responderFilas(w, r, rows, err)
}
func (a *app) reportePerdidas(w http.ResponseWriter, r *http.Request) {
	desde, hasta, err := periodo(r)
	if err != nil {
		escribirError(w, 400, err.Error())
		return
	}
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	rows, err := a.db.QueryContext(ctx, `SELECT motivo,accion_previa,SUM(cantidad) AS unidades,SUM(valor_perdido) AS valor_perdido FROM bajas WHERE fecha_baja>=? AND fecha_baja<DATE_ADD(?,INTERVAL 1 DAY) GROUP BY motivo,accion_previa ORDER BY valor_perdido DESC,motivo,accion_previa`, desde, hasta)
	responderFilas(w, r, rows, err)
}
func (a *app) reporteRotacion(w http.ResponseWriter, r *http.Request) {
	desde, hasta, err := periodo(r)
	if err != nil {
		escribirError(w, 400, err.Error())
		return
	}
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	// Promedio observado del periodo y cobertura del stock ACTUAL: estimación de compra, sin IA.
	rows, err := a.db.QueryContext(ctx, `SELECT p.id AS producto_id,p.nombre,p.stock_minimo,COALESCE(s.stock,0) AS stock,
 COALESCE(d.unidades,0) AS unidades_vendidas,COALESCE(d.total,0) AS total_vendido,
 ROUND(COALESCE(d.unidades,0)/(DATEDIFF(?,?)+1),2) AS promedio_diario,
 CASE WHEN COALESCE(d.unidades,0)=0 THEN NULL ELSE ROUND(COALESCE(s.stock,0)*(DATEDIFF(?,?)+1)/d.unidades,1) END AS dias_cobertura,
 GREATEST(GREATEST(p.stock_minimo,CEIL(COALESCE(d.unidades,0)*30/(DATEDIFF(?,?)+1)))-COALESCE(s.stock,0),0) AS sugerencia_compra_30_dias,
 CASE WHEN COALESCE(d.unidades,0)=0 THEN 'sin_movimiento' WHEN COALESCE(s.stock,0)<=p.stock_minimo THEN 'reponer' ELSE 'con_movimiento' END AS estado,
 d.ultima_venta
 FROM productos p LEFT JOIN (SELECT producto_id,SUM(cantidad_actual) AS stock FROM lotes WHERE fecha_vencimiento>=CURRENT_DATE GROUP BY producto_id) s ON s.producto_id=p.id
 LEFT JOIN (SELECT dv.producto_id,SUM(dv.cantidad) AS unidades,SUM(dv.subtotal) AS total,MAX(v.fecha_hora) AS ultima_venta FROM detalle_venta dv JOIN ventas v ON v.id=dv.venta_id WHERE v.fecha_hora>=? AND v.fecha_hora<DATE_ADD(?,INTERVAL 1 DAY) GROUP BY dv.producto_id) d ON d.producto_id=p.id
 WHERE p.activo=1 ORDER BY unidades_vendidas,p.id`, hasta, desde, hasta, desde, hasta, desde, desde, hasta)
	responderFilas(w, r, rows, err)
}

// Ejecuta alertas aunque no haya navegador abierto. systemd/journald conserva los avisos.
func (a *app) vigilarAlertas(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	anterior := ""
	for {
		c, cancel := context.WithTimeout(ctx, 10*time.Second)
		var bajo, vencido, proximo int64
		err := a.db.QueryRowContext(c, `SELECT
  (SELECT COUNT(*) FROM productos p WHERE p.activo=1 AND COALESCE((SELECT SUM(l.cantidad_actual) FROM lotes l WHERE l.producto_id=p.id AND l.fecha_vencimiento>=CURRENT_DATE),0)<=p.stock_minimo),
  (SELECT COUNT(*) FROM lotes l JOIN productos p ON p.id=l.producto_id WHERE p.activo=1 AND l.cantidad_actual>0 AND l.fecha_vencimiento<CURRENT_DATE),
  (SELECT COUNT(*) FROM lotes l JOIN productos p ON p.id=l.producto_id WHERE p.activo=1 AND l.cantidad_actual>0 AND l.fecha_vencimiento>=CURRENT_DATE AND l.fecha_vencimiento<=DATE_ADD(CURRENT_DATE,INTERVAL p.dias_alerta_temprana DAY))`).Scan(&bajo, &vencido, &proximo)
		cancel()
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("alertas automáticas: %v", err)
			}
		} else {
			estado := fmt.Sprintf("stock_bajo=%d lotes_vencidos=%d lotes_proximos=%d", bajo, vencido, proximo)
			if estado != anterior {
				log.Printf("ALERTAS %s", estado)
				anterior = estado
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
