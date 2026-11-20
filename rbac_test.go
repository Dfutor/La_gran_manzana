package main

import (
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestCatalogoPermisos(t *testing.T) {
	formato := regexp.MustCompile(`^[a-z]+\.[a-z_]+$`)
	vistos := map[string]bool{}
	for _, p := range catalogoPermisos {
		if !formato.MatchString(p.Nombre) || len(p.Nombre) > 60 {
			t.Errorf("nombre inválido: %q", p.Nombre)
		}
		if vistos[p.Nombre] {
			t.Errorf("permiso duplicado: %q", p.Nombre)
		}
		vistos[p.Nombre] = true
		if p.Modulo == "" || p.Descripcion == "" || len(p.Descripcion) > 200 {
			t.Errorf("%s: módulo o descripción inválidos", p.Nombre)
		}
	}
	for _, p := range permisosEmpleado {
		if !vistos[p] {
			t.Errorf("el rol Empleado usa un permiso inexistente: %q", p)
		}
		if indicePermisos[p].Critico {
			t.Errorf("el rol Empleado no debe recibir permisos críticos: %q", p)
		}
	}
}

// Cada ruta /api/ declara un permiso que existe, y cada permiso del catálogo se
// usa en alguna ruta o en un control de alcance: no hay permisos huérfanos.
func TestRutasDeclaranPermisos(t *testing.T) {
	usados := map[string]bool{permVentasVerTodas: true, permBajasVerTodas: true}
	patrones := map[string]bool{}
	for _, r := range (&app{}).rutasAPI() {
		if patrones[r.patron] {
			t.Errorf("ruta repetida: %s", r.patron)
		}
		patrones[r.patron] = true
		if !strings.Contains(r.patron, "/api/") {
			t.Errorf("ruta fuera del API en la tabla de permisos: %s", r.patron)
		}
		switch r.permiso {
		case accesoPublico, accesoSesion:
		default:
			if _, ok := indicePermisos[r.permiso]; !ok {
				t.Errorf("%s exige un permiso inexistente: %q", r.patron, r.permiso)
			}
			usados[r.permiso] = true
		}
	}
	for _, p := range catalogoPermisos {
		if !usados[p.Nombre] {
			t.Errorf("permiso sin uso: %s", p.Nombre)
		}
	}
	for patron, permiso := range permisoDeRuta {
		if permiso == accesoPublico && !slices.Contains([]string{"GET /api/salud", "POST /api/login", "POST /api/logout"}, patron) {
			t.Errorf("ruta pública inesperada: %s", patron)
		}
	}
}

func TestFaltantesYDiferencia(t *testing.T) {
	if f := faltantes(conjunto([]string{"a", "c", "b"}), conjunto([]string{"a"})); !reflect.DeepEqual(f, []string{"b", "c"}) {
		t.Fatal(f)
	}
	if f := faltantes(conjunto(nil), conjunto(nil)); len(f) != 0 {
		t.Fatal(f)
	}
	if f := faltantes(map[string]bool{"x": false}, nil); len(f) != 0 {
		t.Fatal("un valor false no es un permiso requerido", f)
	}
	ag, qu := diferencia([]string{"a", "b"}, []string{"b", "c"})
	if !reflect.DeepEqual(ag, []string{"c"}) || !reflect.DeepEqual(qu, []string{"a"}) {
		t.Fatal(ag, qu)
	}
	if !(usuario{permisos: conjunto([]string{"ventas.ver"})}).tiene("ventas.ver") || (usuario{}).tiene("ventas.ver") {
		t.Fatal("tiene")
	}
}

func TestValidarRol(t *testing.T) {
	texto := func(s string) *string { return &s }
	falso := false
	casos := []struct {
		nombre   string
		in       rolEntrada
		problema string
	}{
		{"sin nombre", rolEntrada{Permisos: []string{}}, "obligatorio"},
		{"nombre en blanco", rolEntrada{Nombre: texto("  "), Permisos: []string{}}, "obligatorio"},
		{"nombre largo", rolEntrada{Nombre: texto(strings.Repeat("á", 61)), Permisos: []string{}}, "60"},
		{"descripción larga", rolEntrada{Nombre: texto("x"), Descripcion: texto(strings.Repeat("a", 256)), Permisos: []string{}}, "255"},
		{"sin lista de permisos", rolEntrada{Nombre: texto("x")}, "permisos es obligatorio"},
		{"permiso inventado", rolEntrada{Nombre: texto("x"), Permisos: []string{"productos.ver", "todo.hacer"}}, "desconocido"},
		{"permiso con mayúsculas", rolEntrada{Nombre: texto("x"), Permisos: []string{"Productos.ver"}}, "desconocido"},
	}
	for _, c := range casos {
		if _, p := validarRol(c.in); !strings.Contains(p, c.problema) {
			t.Errorf("%s: %q", c.nombre, p)
		}
	}
	v, p := validarRol(rolEntrada{Nombre: texto("  Cajero "), Permisos: []string{"ventas.crear", "productos.ver", "ventas.crear"}})
	if p != "" || v.nombre != "Cajero" || !v.activo || !reflect.DeepEqual(v.permisos, []string{"productos.ver", "ventas.crear"}) {
		t.Fatal(v, p)
	}
	v, p = validarRol(rolEntrada{Nombre: texto("Inactivo"), Activo: &falso, Permisos: []string{}})
	if p != "" || v.activo || len(v.permisos) != 0 {
		t.Fatal(v, p)
	}
}

func TestValidarIDsRoles(t *testing.T) {
	if _, p := validarIDsRoles(nil); p == "" {
		t.Error("nil debe exigir la lista")
	}
	if _, p := validarIDsRoles([]int64{1, 0}); p == "" {
		t.Error("admitió id cero")
	}
	if _, p := validarIDsRoles(make([]int64, maxRolesUsuario+1)); p == "" {
		t.Error("admitió demasiados roles")
	}
	if l, p := validarIDsRoles([]int64{3, 1, 3}); p != "" || !reflect.DeepEqual(l, []int64{1, 3}) {
		t.Fatal(l, p)
	}
	if l, p := validarIDsRoles([]int64{}); p != "" || len(l) != 0 {
		t.Fatal(l, p)
	}
}

func TestValidarUsuarioSinRol(t *testing.T) {
	in := usuarioEntrada{Nombre: " Ana ", Email: " ANA@tienda.test ", Password: "ClaveSegura123"}
	if !validarUsuario(&in, true) || in.Estado != "activo" || in.Email != "ana@tienda.test" || in.Nombre != "Ana" {
		t.Fatal(in)
	}
	in = usuarioEntrada{Nombre: "Ana", Email: "ana@tienda.test", Estado: "suspendido"}
	if validarUsuario(&in, false) {
		t.Fatal("admitió estado inválido")
	}
}
