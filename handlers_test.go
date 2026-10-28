package main

import (
	"fmt"
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
		{"POST campo desconocido", "POST", "/api/categorias", `{"nombre":"a","otro":1}`, 400, "campo no permitido"},
		{"POST nombre no es texto", "POST", "/api/categorias", `{"nombre":5}`, 400, "debe ser texto"},
		{"POST dos objetos", "POST", "/api/categorias", `{"nombre":"a"}{"nombre":"b"}`, 400, "un solo objeto"},
		{"PUT categoría sin nombre", "PUT", "/api/categorias/1", `{}`, 400, "obligatorio"},
		{"POST proveedor sin nombre", "POST", "/api/proveedores", `{}`, 400, "obligatorio"},
		{"POST etiqueta tipo inválido", "POST", "/api/etiquetas", `{"nombre":"x","tipo":"desconocido"}`, 400, "tipo inválido"},
		{"POST producto sin categoría", "POST", "/api/productos", `{"nombre":"x"}`, 400, "categoria_id"},
		{"id no numérico", "GET", "/api/productos/abc", "", 400, "entero"},
		{"id cero", "GET", "/api/etiquetas/0/productos", "", 400, "entero"},
		{"tipo inválido", "GET", "/api/etiquetas?tipo=xyz", "", 400, "tipo inválido"},
		{"ruta API inexistente", "GET", "/api/nada", "", 404, "no encontrada"},
		{"método no permitido", "PUT", "/api/categorias", "", 405, "no permitido"},
		{"DELETE sin id es ruta inexistente", "DELETE", "/api/categorias", "", 404, "no encontrada"},
		{"DELETE id inválido", "DELETE", "/api/categorias/-3", "", 400, "entero"},
		{"GET categoría id inválido", "GET", "/api/categorias/x", "", 400, "entero"},
		{"plan FEFO sin cantidad", "GET", "/api/productos/1/plan-fefo", "", 400, "falta el parámetro cantidad"},
		{"plan FEFO cantidad cero", "GET", "/api/productos/1/plan-fefo?cantidad=0", "", 400, "entre 1 y"},
		{"plan FEFO cantidad texto", "GET", "/api/productos/1/plan-fefo?cantidad=diez", "", 400, "entre 1 y"},
		{"plan FEFO cantidad enorme", "GET", "/api/productos/1/plan-fefo?cantidad=1000001", "", 400, "entre 1 y"},
		{"recomendaciones sin necesidad", "GET", "/api/recomendaciones", "", 400, "falta el parámetro necesidad"},
		{"recomendaciones necesidad inválida", "GET", "/api/recomendaciones?necesidad=abc", "", 400, "entero positivo"},
		{"recomendaciones evitar inválido", "GET", "/api/recomendaciones?necesidad=1&evitar=1,x", "", 400, "evitar"},
		{"recomendaciones disponibles inválido", "GET", "/api/recomendaciones?necesidad=1&disponibles=quizas", "", 400, "disponibles"},
		{"búsqueda demasiado larga", "GET", "/api/productos?q=" + strings.Repeat("a", 101), "", 400, "100 caracteres"},
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
			if rec.Header().Get("X-Request-Id") == "" {
				t.Errorf("falta la cabecera X-Request-Id")
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

func TestSugerirAccion(t *testing.T) {
	casos := []struct {
		nivel    string
		acepta   bool
		contiene string
	}{
		{"vencido", true, "baja"},
		{"vencido", false, "baja"},
		{"liquidar", false, "rebaja"},
		{"devolver", true, "devolución"},
		{"devolver", false, "venta destacada"},
		{"otro", true, ""},
	}
	for _, c := range casos {
		got := sugerirAccion(c.nivel, c.acepta)
		if c.contiene == "" && got != "" {
			t.Errorf("sugerirAccion(%q) = %q; se esperaba vacío", c.nivel, got)
		}
		if !strings.Contains(got, c.contiene) {
			t.Errorf("sugerirAccion(%q, %v) = %q; debería contener %q", c.nivel, c.acepta, got, c.contiene)
		}
	}
}

func TestParsearListaIDs(t *testing.T) {
	casos := []struct {
		entrada  string
		want     string
		conError bool
	}{
		{"", "[]", false},
		{"   ", "[]", false},
		{"11", "[11]", false},
		{"11, 13,11", "[11 13]", false},
		{"1,x", "", true},
		{"1,,2", "", true},
		{"0", "", true},
		{"-4", "", true},
		{"1,2,3,4", "", true}, // supera el máximo de 3 usado en la prueba
	}
	for _, c := range casos {
		got, err := parsearListaIDs(c.entrada, 3)
		if (err != nil) != c.conError {
			t.Errorf("parsearListaIDs(%q) error = %v; se esperaba error = %v", c.entrada, err, c.conError)
			continue
		}
		if !c.conError && fmt.Sprint(got) != c.want {
			t.Errorf("parsearListaIDs(%q) = %v; se esperaba %s", c.entrada, got, c.want)
		}
	}
}

func TestPlaceholders(t *testing.T) {
	for n, want := range map[int]string{0: "", 1: "?", 3: "?,?,?"} {
		if got := placeholders(n); got != want {
			t.Errorf("placeholders(%d) = %q; se esperaba %q", n, got, want)
		}
	}
}

func TestPlanificarFEFO(t *testing.T) {
	// Desordenados a propósito: la función debe aplicar FEFO por su cuenta.
	lotes := []loteFEFO{
		{ID: 7, FechaVencimiento: "2027-06-01", CantidadActual: 10, DiasRestantes: 200},
		{ID: 3, FechaVencimiento: "2027-01-15", CantidadActual: 5, DiasRestantes: 80},
		{ID: 9, FechaVencimiento: "2027-01-15", CantidadActual: 4, DiasRestantes: 80},  // empate: gana el id menor (3)
		{ID: 1, FechaVencimiento: "2026-01-01", CantidadActual: 50, DiasRestantes: -5}, // vencido: se ignora
		{ID: 2, FechaVencimiento: "2026-12-01", CantidadActual: 0, DiasRestantes: 40},  // sin stock: se ignora
	}
	type paso struct{ lote, tomar, queda int64 }
	casos := []struct {
		nombre   string
		cantidad int64
		pasos    []paso
		faltante int64
	}{
		{"una unidad sale del primero en vencer", 1, []paso{{3, 1, 4}}, 0},
		{"agota el primer lote exacto", 5, []paso{{3, 5, 0}}, 0},
		{"cruza a el segundo lote (empate por id)", 7, []paso{{3, 5, 0}, {9, 2, 2}}, 0},
		{"usa los tres lotes vendibles", 19, []paso{{3, 5, 0}, {9, 4, 0}, {7, 10, 0}}, 0},
		{"no alcanza", 25, []paso{{3, 5, 0}, {9, 4, 0}, {7, 10, 0}}, 6},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			plan, faltante := planificarFEFO(lotes, c.cantidad)
			if faltante != c.faltante {
				t.Errorf("faltante = %d; se esperaba %d", faltante, c.faltante)
			}
			if len(plan) != len(c.pasos) {
				t.Fatalf("plan con %d lotes; se esperaban %d: %+v", len(plan), len(c.pasos), plan)
			}
			for i, p := range c.pasos {
				if plan[i].LoteID != p.lote || plan[i].Tomar != p.tomar || plan[i].Quedaria != p.queda {
					t.Errorf("paso %d = lote %d toma %d queda %d; se esperaba lote %d toma %d queda %d",
						i, plan[i].LoteID, plan[i].Tomar, plan[i].Quedaria, p.lote, p.tomar, p.queda)
				}
			}
		})
	}
	if plan, faltante := planificarFEFO(nil, 3); len(plan) != 0 || faltante != 3 {
		t.Errorf("sin lotes: plan %v faltante %d; se esperaba [] y 3", plan, faltante)
	}
}

func TestDocumentacionPrivada(t *testing.T) {
	h := (&app{docsUsuario: "usuario", docsClave: "clave"}).rutas()
	casos := []struct {
		nombre, usuario, clave, ruta string
		codigo                       int
	}{
		{"sin credenciales", "", "", "/granmanzana/docs/", 401},
		{"clave incorrecta", "usuario", "otra", "/granmanzana/docs/", 401},
		{"usuario incorrecto", "otro", "clave", "/granmanzana/docs/", 401},
		{"credenciales correctas", "usuario", "clave", "/granmanzana/docs/", 200},
		{"colección con credenciales", "usuario", "clave", "/granmanzana/docs/granmanzana.postman_collection.json", 200},
		{"colección sin credenciales", "", "", "/granmanzana/docs/granmanzana.postman_collection.json", 401},
		{"sin barra final redirige", "", "", "/granmanzana/docs", 301},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, c.ruta, nil)
			if c.usuario != "" {
				req.SetBasicAuth(c.usuario, c.clave)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != c.codigo {
				t.Fatalf("código = %d; se esperaba %d", rec.Code, c.codigo)
			}
			if c.codigo == 401 && !strings.HasPrefix(rec.Header().Get("WWW-Authenticate"), "Basic") {
				t.Errorf("falta WWW-Authenticate: Basic")
			}
		})
	}

	// Sin DOCS_USER/DOCS_PASS configurados, la documentación queda deshabilitada.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/granmanzana/docs/", nil)
	req.SetBasicAuth("", "")
	(&app{}).rutas().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("sin configurar: código = %d; se esperaba 503", rec.Code)
	}
}
