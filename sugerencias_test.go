package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConstruirSugerencias(t *testing.T) {
	si, no := true, false
	prov := "Distribuidora Sol"
	codigo := "L-9"
	venc := []alertaVencimiento{
		{LoteID: 1, ProductoID: 10, Producto: "Té verde", CantidadActual: 3, ValorEnRiesgo: 900, DiasRestantes: -2, Nivel: "vencido", AccionSugerida: "Retirar"},
		{LoteID: 2, ProductoID: 11, Producto: "Miel", CodigoLote: &codigo, CantidadActual: 1, ValorEnRiesgo: 5000, DiasRestantes: 0, Nivel: "liquidar"},
		{LoteID: 3, ProductoID: 12, Producto: "Avena", CantidadActual: 4, DiasRestantes: 90, Nivel: "devolver", Proveedor: &prov, AceptaDevolucion: &si},
		{LoteID: 4, ProductoID: 13, Producto: "Chía", CantidadActual: 2, DiasRestantes: 80, Nivel: "devolver", AceptaDevolucion: &no},
	}
	stock := []alertaStock{
		{productoResumen: productoResumen{ID: 20, Nombre: "Linaza", Stock: 0, StockMinimo: 5}, FaltanteParaMinimo: 5},
		{productoResumen: productoResumen{ID: 21, Nombre: "Quinua", Stock: 2, StockMinimo: 5}, FaltanteParaMinimo: 3},
	}
	l := construirSugerencias(venc, stock)
	if len(l) != 6 {
		t.Fatal(len(l))
	}
	// Prioridad 1 primero y, dentro de ella, el mayor valor en riesgo.
	if l[0].Clave != "lote:2:liquidar" || l[1].Clave != "lote:1:vencido" || l[2].Clave != "stock:20:0" {
		t.Fatal(l[0].Clave, l[1].Clave, l[2].Clave)
	}
	tipos := map[string]sugerencia{}
	for _, s := range l {
		tipos[s.Clave] = s
		if s.Titulo == "" || s.Motivo == "" || s.Accion == "" || s.Destino == "" {
			t.Errorf("sugerencia incompleta: %+v", s)
		}
	}
	if s := tipos["lote:3:devolver"]; s.Tipo != "devolver" || s.Destino != "baja" || !strings.Contains(s.Titulo, prov) {
		t.Error(s)
	}
	if s := tipos["lote:4:devolver"]; s.Tipo != "destacar" || s.Destino != "producto" {
		t.Error(s)
	}
	if s := tipos["lote:2:liquidar"]; !strings.Contains(s.Motivo, "hoy") || !strings.Contains(s.Motivo, "L-9") || !strings.Contains(s.Motivo, "1 unidad ") {
		t.Error(s.Motivo)
	}
	if s := tipos["lote:1:vencido"]; !strings.Contains(s.Motivo, "2 días") {
		t.Error(s.Motivo)
	}
	if len(construirSugerencias(nil, nil)) != 0 {
		t.Error("sin alertas no hay sugerencias")
	}
}

// Las rutas de la aplicación devuelven index.html; los recursos inexistentes, 404.
func TestRutasSPA(t *testing.T) {
	h := (&app{}).enrutador()
	for ruta, esperado := range map[string]int{
		"/granmanzana/": 200, "/granmanzana/login": 200, "/granmanzana/admin/login": 200, "/granmanzana/app/ventas": 200,
		"/granmanzana/admin/usuarios/7": 200, "/granmanzana/js/main.js": 200, "/granmanzana/css/tokens.css": 200,
		"/granmanzana/js/no-existe.js": 404, "/granmanzana/js": 200,
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", ruta, nil))
		if rec.Code != esperado {
			t.Errorf("%s = %d; se esperaba %d", ruta, rec.Code, esperado)
			continue
		}
		cuerpo := rec.Body.String()
		esHTML := strings.Contains(cuerpo, `<script type="module" src="js/main.js">`)
		if esperado == 200 && !strings.Contains(ruta, ".") && !esHTML {
			t.Errorf("%s no devolvió la aplicación", ruta)
		}
		if strings.HasSuffix(ruta, ".js") && esperado == 200 && (esHTML || !strings.Contains(rec.Header().Get("Content-Type"), "javascript")) {
			t.Errorf("%s debería servir JavaScript, no %q", ruta, rec.Header().Get("Content-Type"))
		}
	}
}
