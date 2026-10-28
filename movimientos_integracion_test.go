package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func productoPrueba(t *testing.T, a *app, srv *httptest.Server) int64 {
	t.Helper()
	var c struct{ ID int64 }
	if estado := pedir(t, "POST", srv.URL+"/api/categorias", fmt.Sprintf(`{"nombre":"Movimientos %d"}`, time.Now().UnixNano()), &c); estado != 201 {
		t.Fatal(estado)
	}
	var p struct{ ID int64 }
	if estado := pedir(t, "POST", srv.URL+"/api/productos", fmt.Sprintf(`{"nombre":"Producto transaccional","categoria_id":%d,"precio_venta":0.10,"etiquetas":[]}`, c.ID), &p); estado != 201 {
		t.Fatal(estado)
	}
	t.Cleanup(func() {
		a.db.Exec(`DELETE FROM detalle_venta WHERE producto_id=?`, p.ID)
		a.db.Exec(`DELETE v FROM ventas v LEFT JOIN detalle_venta d ON d.venta_id=v.id WHERE d.id IS NULL`)
		a.db.Exec(`DELETE FROM bajas WHERE producto_id=?`, p.ID)
		a.db.Exec(`DELETE FROM lotes WHERE producto_id=?`, p.ID)
		a.db.Exec(`DELETE FROM productos WHERE id=?`, p.ID)
		a.db.Exec(`DELETE FROM categorias WHERE id=?`, c.ID)
	})
	return p.ID
}
func ingresarPrueba(t *testing.T, srv *httptest.Server, p int64, dias, cantidad int) int64 {
	t.Helper()
	var l struct{ ID int64 }
	fecha := time.Now().AddDate(0, 0, dias).Format(formatoFecha)
	estado := pedir(t, "POST", srv.URL+"/api/lotes", fmt.Sprintf(`{"producto_id":%d,"fecha_vencimiento":%q,"cantidad":%d,"costo_unitario":"0.05"}`, p, fecha, cantidad), &l)
	if estado != 201 {
		t.Fatal("ingreso", estado)
	}
	return l.ID
}
func TestIntegracionVentaFEFOYRollback(t *testing.T) {
	a, srv := appIntegracion(t)
	p := productoPrueba(t, a, srv)
	tardio := ingresarPrueba(t, srv, p, 90, 4)
	temprano := ingresarPrueba(t, srv, p, 10, 2)
	res, err := a.db.Exec(`INSERT INTO lotes(producto_id,fecha_vencimiento,cantidad_inicial,cantidad_actual,costo_unitario) VALUES(?,CURRENT_DATE-INTERVAL 1 DAY,10,10,0.05)`, p)
	if err != nil {
		t.Fatal(err)
	}
	vencido, _ := res.LastInsertId()
	var venta struct {
		ID       int64
		Total    dinero
		Detalles []lineaVenta
	}
	cuerpo := fmt.Sprintf(`{"medio_pago":"nequi","detalles":[{"producto_id":%d,"cantidad":3}]}`, p)
	if c := pedir(t, "POST", srv.URL+"/api/ventas", cuerpo, &venta); c != 201 {
		t.Fatal(c)
	}
	if venta.Total != 30 || len(venta.Detalles) != 1 || len(venta.Detalles[0].Asignacion) != 2 || venta.Detalles[0].Asignacion[0].LoteID != temprano || venta.Detalles[0].Asignacion[1].LoteID != tardio {
		t.Fatalf("FEFO/totales incorrectos: %+v", venta)
	}
	for id, want := range map[int64]int{temprano: 0, tardio: 3, vencido: 10} {
		var stock int
		a.db.QueryRow(`SELECT cantidad_actual FROM lotes WHERE id=?`, id).Scan(&stock)
		if stock != want {
			t.Errorf("lote %d stock=%d esperado=%d", id, stock, want)
		}
	}
	// Primer producto se alcanza a descontar, segundo falla: revierte TODO.
	otro := productoPrueba(t, a, srv)
	cuerpo = fmt.Sprintf(`{"medio_pago":"efectivo","detalles":[{"producto_id":%d,"cantidad":1},{"producto_id":%d,"cantidad":1}]}`, p, otro)
	if c := pedir(t, "POST", srv.URL+"/api/ventas", cuerpo, nil); c != 409 {
		t.Fatal(c)
	}
	var stock, cuenta int
	a.db.QueryRow(`SELECT cantidad_actual FROM lotes WHERE id=?`, tardio).Scan(&stock)
	a.db.QueryRow(`SELECT COUNT(*) FROM detalle_venta WHERE producto_id=?`, p).Scan(&cuenta)
	if stock != 3 || cuenta != 1 {
		t.Fatalf("rollback incompleto: stock %d detalles %d", stock, cuenta)
	}
	cuerpo = fmt.Sprintf(`{"medio_pago":"daviplata","detalles":[{"producto_id":%d,"cantidad":4}]}`, p)
	if c := pedir(t, "POST", srv.URL+"/api/ventas", cuerpo, nil); c != 409 {
		t.Fatal("vendió vencidos", c)
	}
	var historica struct {
		Total    dinero
		Detalles []struct {
			Precio dinero `json:"precio_unitario"`
		}
	}
	a.db.Exec(`UPDATE productos SET precio_venta=999 WHERE id=?`, p)
	if c := pedir(t, "GET", fmt.Sprintf("%s/api/ventas/%d", srv.URL, venta.ID), "", &historica); c != 200 || historica.Total != 30 || historica.Detalles[0].Precio != 10 {
		t.Fatal("precio histórico", c, historica)
	}
	for _, ruta := range []string{"/api/reportes/ventas", "/api/reportes/ventas?agrupar=producto", "/api/reportes/rotacion", "/api/reportes/perdidas", "/api/lotes", "/api/ventas", "/api/bajas"} {
		var filas []map[string]any
		if c := pedir(t, "GET", srv.URL+ruta, "", &filas); c != 200 {
			t.Errorf("%s: %d", ruta, c)
		}
	}
}
func TestIntegracionConcurrenciaYBajas(t *testing.T) {
	a, srv := appIntegracion(t)
	p := productoPrueba(t, a, srv)
	l := ingresarPrueba(t, srv, p, 30, 5)
	cuerpo := fmt.Sprintf(`{"medio_pago":"efectivo","detalles":[{"producto_id":%d,"cantidad":4}]}`, p)
	cookie, _ := clientesIntegracion.Load(srv.URL)
	resultados := make(chan int, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest("POST", srv.URL+"/api/ventas", bytes.NewBufferString(cuerpo))
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(cookie.(*http.Cookie))
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				resultados <- 0
				return
			}
			defer resp.Body.Close()
			resultados <- resp.StatusCode
		}()
	}
	wg.Wait()
	close(resultados)
	codigos := map[int]int{}
	for c := range resultados {
		codigos[c]++
	}
	if codigos[201] != 1 || codigos[409] != 1 {
		t.Fatal(codigos)
	}
	var stock int
	a.db.QueryRow(`SELECT cantidad_actual FROM lotes WHERE id=?`, l).Scan(&stock)
	if stock != 1 {
		t.Fatal(stock)
	}
	baja := fmt.Sprintf(`{"lote_id":%d,"cantidad":1,"motivo":"vencimiento"}`, l)
	if c := pedir(t, "POST", srv.URL+"/api/bajas", baja, nil); c != 400 {
		t.Fatal("baja por vencimiento anticipada", c)
	}
	baja = fmt.Sprintf(`{"lote_id":%d,"cantidad":1,"motivo":"deterioro","accion_previa":"rebaja"}`, l)
	var b struct {
		ID    int64
		Valor dinero `json:"valor_perdido"`
	}
	if c := pedir(t, "POST", srv.URL+"/api/bajas", baja, &b); c != 201 || b.Valor != 5 {
		t.Fatal(c, b)
	}
	if c := pedir(t, "POST", srv.URL+"/api/bajas", baja, nil); c != 409 {
		t.Fatal("baja duplicada", c)
	}
	a.db.QueryRow(`SELECT cantidad_actual FROM lotes WHERE id=?`, l).Scan(&stock)
	if stock != 0 {
		t.Fatal(stock)
	}
}
func TestIntegracionLoginYPermisos(t *testing.T) {
	a, srv := appIntegracion(t)
	email := fmt.Sprintf("empleado%d@example.test", time.Now().UnixNano())
	var u usuario
	cuerpo, _ := json.Marshal(map[string]string{"nombre": "Empleado prueba", "email": email, "password": "ClaveEmpleado123!", "rol": "empleado"})
	if c := pedir(t, "POST", srv.URL+"/api/usuarios", string(cuerpo), &u); c != 201 {
		t.Fatal(c)
	}
	t.Cleanup(func() { a.db.Exec(`DELETE FROM usuarios WHERE id=?`, u.ID) })
	cuerpo, _ = json.Marshal(map[string]string{"email": email, "password": "ClaveEmpleado123!"})
	resp, err := http.Post(srv.URL+"/api/login", "application/json", bytes.NewReader(cuerpo))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
	c := resp.Cookies()[0]
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/api/" {
		t.Fatal(c)
	}
	solicitar := func(metodo, ruta string) int {
		req, _ := http.NewRequest(metodo, srv.URL+ruta, nil)
		req.AddCookie(c)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}
	if code := solicitar("GET", "/api/usuarios"); code != 403 {
		t.Fatal(code)
	}
	if code := solicitar("POST", "/api/productos"); code != 403 {
		t.Fatal(code)
	}
	if code := solicitar("GET", "/api/productos"); code != 200 {
		t.Fatal(code)
	}
	if code := solicitar("GET", "/api/reportes/ventas"); code != 403 {
		t.Fatal(code)
	}
	a.db.Exec(`UPDATE usuarios SET estado='inactivo' WHERE id=?`, u.ID)
	if code := solicitar("GET", "/api/productos"); code != 401 {
		t.Fatal(code)
	}
	a.db.Exec(`UPDATE usuarios SET estado='activo',password_hash='HASH_DE_EJEMPLO_CAMBIAR' WHERE id=?`, u.ID)
	if code := solicitar("GET", "/api/productos"); code != 401 {
		t.Fatal(code)
	}
	resp, err = http.Post(srv.URL+"/api/login", "application/json", bytes.NewReader(cuerpo))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatal("aceptó marcador de hash", resp.StatusCode)
	}
}
