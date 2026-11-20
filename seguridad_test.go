package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAPISinSesion(t *testing.T) {
	a := &app{}
	h := a.rutas()
	for _, ruta := range []string{"/api/productos", "/api/usuarios", "/api/ventas", "/api/lotes", "/api/bajas", "/api/reportes/rotacion", "/api/sesion"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", ruta, nil))
		if rec.Code != 401 {
			t.Errorf("%s = %d", ruta, rec.Code)
		}
	}
	for _, origen := range []string{"https://ajeno.test", "null"} {
		req := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{}`))
		req.Header.Set("Origin", origen)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 403 {
			t.Errorf("origen %s = %d", origen, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/sesion", nil)
	req.AddCookie(&http.Cookie{Name: nombreCookie, Value: "inventada"})
	h.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatal(rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/logout", nil))
	if rec.Code != 204 || len(rec.Result().Cookies()) != 1 || rec.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("logout debe borrar la cookie")
	}
}
func TestPermisos(t *testing.T) {
	mux := (&app{}).enrutador()
	for _, c := range []struct{ metodo, ruta, permiso string }{
		{"POST", "/api/ventas", "ventas.crear"}, {"POST", "/api/bajas", "bajas.crear"}, {"POST", "/api/lotes", "lotes.crear"},
		{"PUT", "/api/lotes/1", "lotes.editar"}, {"GET", "/api/reportes/ventas", "reportes.ventas"}, {"GET", "/api/usuarios", "usuarios.ver"},
		{"POST", "/api/productos", "productos.crear"}, {"DELETE", "/api/categorias/1", "categorias.eliminar"}, {"GET", "/api/productos", "productos.ver"},
		{"HEAD", "/api/productos/3", "productos.ver"}, {"PUT", "/api/usuarios/2/roles", "roles.asignar"}, {"DELETE", "/api/roles/4", "roles.gestionar"},
		{"GET", "/api/usuarios/2/permisos", "usuarios.ver"}, {"GET", "/api/auditoria", "auditoria.ver"},
		{"POST", "/api/login", accesoPublico}, {"GET", "/api/sesion", accesoSesion}, {"GET", "/api/nada", accesoSesion}, {"PUT", "/api/ventas", accesoSesion},
	} {
		_, patron := mux.Handler(httptest.NewRequest(c.metodo, c.ruta, nil))
		if got, ok := permisoDeRuta[patron]; !ok || got != c.permiso {
			t.Errorf("%s %s (patrón %q) = %q; se esperaba %q", c.metodo, c.ruta, patron, got, c.permiso)
		}
	}
}
func TestLimiteLogin(t *testing.T) {
	a := nuevaAutenticacion()
	for i := 0; i < 10; i++ {
		if !a.permitirLogin("ip") {
			t.Fatal(i)
		}
	}
	if a.permitirLogin("ip") {
		t.Fatal("admitió intento once")
	}
	a.intentos["ip"] = intentoLogin{10, time.Now().Add(-time.Second)}
	if !a.permitirLogin("ip") {
		t.Fatal("no renovó ventana")
	}
}
func TestImportes(t *testing.T) {
	for _, s := range []string{"-1", "1.001", "1e3", "null", "99999999999.00", "NaN", "1.", ".2"} {
		var d dinero
		if json.Unmarshal([]byte(s), &d) == nil {
			t.Errorf("admitió %s", s)
		}
	}
	for _, c := range []struct {
		json     string
		centavos dinero
	}{{"0.10", 10}, {`"12.34"`, 1234}, {"9999999999.99", maxDinero}, {"1", 100}} {
		var d dinero
		if err := json.Unmarshal([]byte(c.json), &d); err != nil || d != c.centavos {
			t.Errorf("%s = %v %v", c.json, d, err)
		}
	}
	var p dinero
	json.Unmarshal([]byte("0.10"), &p)
	b, _ := json.Marshal(p * 3)
	if string(b) != "0.30" {
		t.Fatal(string(b))
	}
}
func TestValidacionMovimientos(t *testing.T) {
	for _, s := range []string{`{"medio_pago":"tarjeta","detalles":[{"producto_id":1,"cantidad":1}]}`, `{"medio_pago":"efectivo","detalles":[]}`, `{"medio_pago":"nequi","detalles":[{"producto_id":1,"cantidad":1},{"producto_id":1,"cantidad":2}]}`, `{"medio_pago":"efectivo","detalles":[{"producto_id":1,"cantidad":-1}]}`} {
		var v ventaEntrada
		json.Unmarshal([]byte(s), &v)
		if validarVenta(&v) == nil {
			t.Error(s)
		}
	}
	v := ventaEntrada{"efectivo", []lineaVentaEntrada{{ProductoID: 2, Cantidad: 1}, {ProductoID: 1, Cantidad: 1}}}
	if err := validarVenta(&v); err != nil || v.Detalles[0].ProductoID != 1 {
		t.Fatal(v, err)
	}
	b := bajaEntrada{1, 1, "vencimiento", ""}
	if validarBaja(&b) != nil || b.AccionPrevia != "ninguna" {
		t.Fatal(b)
	}
	for _, s := range []string{"2026-02-30", "mañana", ""} {
		c := dinero(0)
		if validarLote(loteEntrada{1, nil, nil, s, 1, &c}) == nil {
			t.Error(s)
		}
	}
	for _, s := range []string{"/api/reportes/ventas?desde=2026-12-01&hasta=2026-01-01", "/api/reportes/ventas?desde=no", "/api/reportes/ventas?desde=2020-01-01&hasta=2026-01-01"} {
		if _, _, err := periodo(httptest.NewRequest("GET", s, nil)); err == nil {
			t.Error(s)
		}
	}
}

func TestHostProxyLocal(t *testing.T) {
	req := httptest.NewRequest("POST", "http://127.0.0.1:8080/api/login", nil)
	req.Header.Set("X-Forwarded-Host", "falso.test, tienda.test")
	req.RemoteAddr = "127.0.0.1:1234"
	if got := hostPublico(req); got != "tienda.test" {
		t.Fatal(got)
	}
	req.RemoteAddr = "203.0.113.1:1234"
	if got := hostPublico(req); got != "127.0.0.1:8080" {
		t.Fatal(got)
	}
}
