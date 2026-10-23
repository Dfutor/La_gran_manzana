package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClasificarVencimiento(t *testing.T) {
	const limite = 60
	casos := []struct {
		nombre string
		dias   int
		want   string
	}{
		{"vencido ayer", -1, "vencido"},
		{"vencido hace mucho", -30, "vencido"},
		{"vence hoy", 0, "liquidar"},
		{"dentro de liquidación", 30, "liquidar"},
		{"igual al límite", limite, "liquidar"},
		{"límite + 1", limite + 1, "devolver"},
		{"lejos del vencimiento", 119, "devolver"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := clasificarVencimiento(c.dias, limite); got != c.want {
				t.Errorf("clasificarVencimiento(%d, %d) = %q; se esperaba %q", c.dias, limite, got, c.want)
			}
		})
	}
}

func TestClasificarVencimientoLiquidacionCero(t *testing.T) {
	casos := map[int]string{-1: "vencido", 0: "liquidar", 1: "devolver"}
	for dias, want := range casos {
		if got := clasificarVencimiento(dias, 0); got != want {
			t.Errorf("clasificarVencimiento(%d, 0) = %q; se esperaba %q", dias, got, want)
		}
	}
}

func TestValidarNombreCategoria(t *testing.T) {
	casos := []struct {
		nombre   string
		entrada  string
		want     string
		conError bool
	}{
		{"normal", "Superalimentos", "Superalimentos", false},
		{"recorta espacios", "  Tés  ", "Tés", false},
		{"vacío", "", "", true},
		{"solo espacios", "   ", "", true},
		{"80 caracteres con tildes", strings.Repeat("á", 80), strings.Repeat("á", 80), false},
		{"81 caracteres", strings.Repeat("a", 81), "", true},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got, problema := validarNombreCategoria(c.entrada)
			if (problema != "") != c.conError {
				t.Fatalf("problema = %q; se esperaba error = %v", problema, c.conError)
			}
			if got != c.want {
				t.Errorf("nombre = %q; se esperaba %q", got, c.want)
			}
		})
	}
}

func TestEscaparLike(t *testing.T) {
	casos := map[string]string{
		"menta":   "menta",
		"50%":     `50\%`,
		"a_b":     `a\_b`,
		`c:\ruta`: `c:\\ruta`,
	}
	for entrada, want := range casos {
		if got := escaparLike(entrada); got != want {
			t.Errorf("escaparLike(%q) = %q; se esperaba %q", entrada, got, want)
		}
	}
}

// Estas pruebas no tocan la base: las validaciones responden antes de consultarla.
func TestRutasSinBaseDeDatos(t *testing.T) {
	h := (&app{}).rutas()
	casos := []struct {
		nombre, metodo, ruta, cuerpo string
		codigo                       int
		contiene                     string
	}{
		{"POST vacío", "POST", "/api/categorias", "", 400, "vacío"},
		{"POST JSON inválido", "POST", "/api/categorias", "{nombre:", 400, "JSON inválido"},
		{"POST sin nombre", "POST", "/api/categorias", `{}`, 400, "obligatorio"},
		{"POST nombre en blanco", "POST", "/api/categorias", `{"nombre":"   "}`, 400, "obligatorio"},
		{"POST nombre largo", "POST", "/api/categorias", `{"nombre":"` + strings.Repeat("x", 81) + `"}`, 400, "80"},
		{"POST campo desconocido", "POST", "/api/categorias", `{"nombre":"a","otro":1}`, 400, "JSON inválido"},
		{"id no numérico", "GET", "/api/productos/abc", "", 400, "entero"},
		{"id cero", "GET", "/api/etiquetas/0/productos", "", 400, "entero"},
		{"tipo inválido", "GET", "/api/etiquetas?tipo=xyz", "", 400, "tipo inválido"},
		{"ruta API inexistente", "GET", "/api/nada", "", 404, "no encontrada"},
		{"método no permitido", "DELETE", "/api/categorias", "", 405, ""},
		{"preflight CORS", "OPTIONS", "/api/categorias", "", 204, ""},
		{"raíz redirige", "GET", "/", "", 302, ""},
		{"frontend", "GET", "/granmanzana/", "", 200, "La Gran Manzana"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			req := httptest.NewRequest(c.metodo, c.ruta, strings.NewReader(c.cuerpo))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != c.codigo {
				t.Fatalf("código = %d; se esperaba %d (cuerpo: %s)", rec.Code, c.codigo, rec.Body.String())
			}
			if c.contiene != "" && !strings.Contains(rec.Body.String(), c.contiene) {
				t.Errorf("el cuerpo %q no contiene %q", rec.Body.String(), c.contiene)
			}
			if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
				t.Errorf("falta la cabecera CORS")
			}
		})
	}
}

func TestRedireccionRaiz(t *testing.T) {
	rec := httptest.NewRecorder()
	(&app{}).rutas().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if loc := rec.Header().Get("Location"); loc != "/granmanzana/" {
		t.Errorf("Location = %q; se esperaba /granmanzana/", loc)
	}
}
