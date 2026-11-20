package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

func rolPorNombre(t *testing.T, srv *httptest.Server, nombre string) int64 {
	t.Helper()
	var roles []rolDetalle
	if c := pedir(t, "GET", srv.URL+"/api/roles", "", &roles); c != 200 {
		t.Fatal("GET /api/roles", c)
	}
	for _, r := range roles {
		if r.Nombre == nombre {
			return r.ID
		}
	}
	t.Fatalf("no existe el rol %q", nombre)
	return 0
}

func iniciarSesion(t *testing.T, srv *httptest.Server, email, clave string) *http.Cookie {
	t.Helper()
	cuerpo, _ := json.Marshal(map[string]string{"email": email, "password": clave})
	resp, err := http.Post(srv.URL+"/api/login", "application/json", bytes.NewReader(cuerpo))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || len(resp.Cookies()) == 0 {
		t.Fatal("login", email, resp.StatusCode)
	}
	return resp.Cookies()[0]
}

// pedirComo hace la petición con la sesión indicada en lugar de la del propietario de prueba.
func pedirComo(t *testing.T, c *http.Cookie, metodo, url, cuerpo string, destino any) int {
	t.Helper()
	req, err := http.NewRequest(metodo, url, strings.NewReader(cuerpo))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(c)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if destino != nil && resp.StatusCode < 300 && resp.StatusCode != 204 {
		if err := json.NewDecoder(resp.Body).Decode(destino); err != nil {
			t.Fatal(err)
		}
	}
	return resp.StatusCode
}

func crearRolPrueba(t *testing.T, a *app, srv *httptest.Server, nombre string, permisos ...string) int64 {
	t.Helper()
	cuerpo, _ := json.Marshal(map[string]any{"nombre": nombre, "descripcion": "prueba", "permisos": permisos})
	var r rolDetalle
	if c := pedir(t, "POST", srv.URL+"/api/roles", string(cuerpo), &r); c != 201 || !slices.Equal(r.Permisos, ordenados(conjunto(permisos))) {
		t.Fatal("crear rol", nombre, c, r)
	}
	t.Cleanup(func() {
		a.db.Exec(`DELETE FROM usuario_roles WHERE rol_id=?`, r.ID)
		a.db.Exec(`DELETE FROM roles WHERE id=?`, r.ID)
	})
	return r.ID
}

func crearUsuarioPrueba(t *testing.T, a *app, srv *httptest.Server, prefijo string) (int64, string) {
	t.Helper()
	email := fmt.Sprintf("%s%d@example.test", prefijo, time.Now().UnixNano())
	var u usuario
	cuerpo, _ := json.Marshal(map[string]string{"nombre": "RBAC " + prefijo, "email": email, "password": "ClaveRBAC12345!"})
	if c := pedir(t, "POST", srv.URL+"/api/usuarios", string(cuerpo), &u); c != 201 {
		t.Fatal("crear usuario", c)
	}
	t.Cleanup(func() {
		a.db.Exec(`DELETE d FROM detalle_venta d JOIN ventas v ON v.id=d.venta_id WHERE v.usuario_id=?`, u.ID)
		a.db.Exec(`DELETE FROM ventas WHERE usuario_id=?`, u.ID)
		a.db.Exec(`DELETE FROM usuarios WHERE id=?`, u.ID)
	})
	return u.ID, email
}

func asignar(t *testing.T, c *http.Cookie, srv *httptest.Server, usuarioID int64, roles ...int64) int {
	t.Helper()
	cuerpo, _ := json.Marshal(map[string]any{"roles": append([]int64{}, roles...)})
	url := fmt.Sprintf("%s/api/usuarios/%d/roles", srv.URL, usuarioID)
	if c == nil {
		return pedir(t, "PUT", url, string(cuerpo), nil)
	}
	return pedirComo(t, c, "PUT", url, string(cuerpo), nil)
}

func TestIntegracionRBAC(t *testing.T) {
	a, srv := appIntegracion(t)
	sufijo := time.Now().UnixNano()
	propietario := rolPorNombre(t, srv, nombreRolPropietario)
	var yo perfilUsuario
	if c := pedir(t, "GET", srv.URL+"/api/sesion", "", &yo); c != 200 || len(yo.Permisos) != len(catalogoPermisos) {
		t.Fatal("el propietario debe tener todo el catálogo", c, len(yo.Permisos))
	}

	// Usuario nuevo: sin roles no tiene acceso a nada salvo su sesión.
	cajeroID, cajeroEmail := crearUsuarioPrueba(t, a, srv, "cajero")
	cajero := iniciarSesion(t, srv, cajeroEmail, "ClaveRBAC12345!")
	if c := pedirComo(t, cajero, "GET", srv.URL+"/api/productos", "", nil); c != 403 {
		t.Fatal("sin roles debería ser 403", c)
	}

	// Asignar un rol personalizado aplica en la siguiente petición, sin volver a ingresar.
	rolCajero := crearRolPrueba(t, a, srv, fmt.Sprintf("Cajero %d", sufijo), "productos.ver", "ventas.ver", "ventas.crear")
	if c := asignar(t, nil, srv, cajeroID, rolCajero); c != 200 {
		t.Fatal("asignar", c)
	}
	for ruta, esperado := range map[string]int{"/api/productos": 200, "/api/categorias": 403, "/api/usuarios": 403, "/api/reportes/ventas": 403} {
		if c := pedirComo(t, cajero, "GET", srv.URL+ruta, "", nil); c != esperado {
			t.Errorf("cajero GET %s = %d; se esperaba %d", ruta, c, esperado)
		}
	}
	if c := pedirComo(t, cajero, "POST", srv.URL+"/api/categorias", `{"nombre":"no"}`, nil); c != 403 {
		t.Error("cajero creó categoría", c)
	}

	// Alcance: sin ventas.ver_todas solo ve sus ventas.
	producto := productoPrueba(t, a, srv)
	ingresarPrueba(t, srv, producto, 30, 5)
	venta := fmt.Sprintf(`{"medio_pago":"efectivo","detalles":[{"producto_id":%d,"cantidad":1}]}`, producto)
	var ajena, propia struct{ ID int64 }
	if c := pedir(t, "POST", srv.URL+"/api/ventas", venta, &ajena); c != 201 {
		t.Fatal("venta propietario", c)
	}
	if c := pedirComo(t, cajero, "POST", srv.URL+"/api/ventas", venta, &propia); c != 201 {
		t.Fatal("venta cajero", c)
	}
	var lista []map[string]any
	if c := pedirComo(t, cajero, "GET", srv.URL+"/api/ventas?limite=500", "", &lista); c != 200 || len(lista) == 0 {
		t.Fatal("ventas cajero", c, len(lista))
	}
	for _, v := range lista {
		if fmt.Sprint(v["usuario_id"]) != fmt.Sprint(cajeroID) {
			t.Fatalf("el cajero ve una venta ajena: %v", v)
		}
	}
	if c := pedirComo(t, cajero, "GET", fmt.Sprintf("%s/api/ventas/%d", srv.URL, ajena.ID), "", nil); c != 404 {
		t.Error("detalle de venta ajena", c)
	}
	if c := pedirComo(t, cajero, "GET", fmt.Sprintf("%s/api/ventas/%d", srv.URL, propia.ID), "", nil); c != 200 {
		t.Error("detalle de venta propia", c)
	}
	if c := pedir(t, "GET", fmt.Sprintf("%s/api/ventas/%d", srv.URL, propia.ID), "", nil); c != 200 {
		t.Error("el propietario ve todas las ventas", c)
	}

	// Editar el rol cambia el acceso de todos sus usuarios; desactivarlo lo revoca.
	editar := func(activo bool, permisos ...string) int {
		cuerpo, _ := json.Marshal(map[string]any{"nombre": fmt.Sprintf("Cajero %d", sufijo), "activo": activo, "permisos": permisos})
		return pedir(t, "PUT", fmt.Sprintf("%s/api/roles/%d", srv.URL, rolCajero), string(cuerpo), nil)
	}
	if c := editar(true, "productos.ver", "ventas.ver", "ventas.crear", "categorias.ver"); c != 200 {
		t.Fatal("editar rol", c)
	}
	if c := pedirComo(t, cajero, "GET", srv.URL+"/api/categorias", "", nil); c != 200 {
		t.Error("permiso agregado no aplicó", c)
	}
	if c := editar(false, "productos.ver", "ventas.ver", "ventas.crear", "categorias.ver"); c != 200 {
		t.Fatal("desactivar rol", c)
	}
	if c := pedirComo(t, cajero, "GET", srv.URL+"/api/productos", "", nil); c != 403 {
		t.Error("rol desactivado sigue otorgando permisos", c)
	}
	if c := editar(true, "productos.ver", "ventas.ver", "ventas.crear"); c != 200 {
		t.Fatal("reactivar rol", c)
	}

	// Permisos efectivos con su origen.
	var efectivos struct {
		Permisos []permisoEfectivo `json:"permisos"`
	}
	if c := pedir(t, "GET", fmt.Sprintf("%s/api/usuarios/%d/permisos", srv.URL, cajeroID), "", &efectivos); c != 200 || len(efectivos.Permisos) != 3 || efectivos.Permisos[0].Roles[0] != fmt.Sprintf("Cajero %d", sufijo) {
		t.Fatal("permisos efectivos", c, efectivos)
	}

	// Administrador delegado: gestiona roles pero no puede escalar.
	rolGestor := crearRolPrueba(t, a, srv, fmt.Sprintf("Gestor %d", sufijo), "roles.ver", "roles.gestionar", "roles.asignar", "usuarios.ver", "productos.ver")
	gestorID, gestorEmail := crearUsuarioPrueba(t, a, srv, "gestor")
	if c := asignar(t, nil, srv, gestorID, rolGestor); c != 200 {
		t.Fatal(c)
	}
	gestor := iniciarSesion(t, srv, gestorEmail, "ClaveRBAC12345!")
	if c := pedirComo(t, gestor, "POST", srv.URL+"/api/roles", fmt.Sprintf(`{"nombre":"Escalada %d","permisos":["reportes.ventas"]}`, sufijo), nil); c != 403 {
		t.Error("otorgó un permiso que no tiene", c)
	}
	var menor rolDetalle
	if c := pedirComo(t, gestor, "POST", srv.URL+"/api/roles", fmt.Sprintf(`{"nombre":"Consulta %d","permisos":["productos.ver"]}`, sufijo), &menor); c != 201 {
		t.Error("no pudo crear un rol con permisos propios", c)
	}
	t.Cleanup(func() {
		a.db.Exec(`DELETE FROM usuario_roles WHERE rol_id=?`, menor.ID)
		a.db.Exec(`DELETE FROM roles WHERE id=?`, menor.ID)
	})
	casos := []struct {
		nombre         string
		metodo, ruta   string
		cuerpo         string
		codigoEsperado int
	}{
		{"editar el rol propio", "PUT", fmt.Sprintf("/api/roles/%d", rolGestor), `{"nombre":"x","permisos":[]}`, 409},
		{"editar el rol del sistema", "PUT", fmt.Sprintf("/api/roles/%d", propietario), `{"nombre":"x","permisos":[]}`, 409},
		{"eliminar el rol del sistema", "DELETE", fmt.Sprintf("/api/roles/%d", propietario), "", 409},
		{"cambiarse sus roles", "PUT", fmt.Sprintf("/api/usuarios/%d/roles", gestorID), fmt.Sprintf(`{"roles":[%d,%d]}`, rolGestor, propietario), 409},
		{"asignar Propietario a otro", "PUT", fmt.Sprintf("/api/usuarios/%d/roles", cajeroID), fmt.Sprintf(`{"roles":[%d]}`, propietario), 403},
		{"quitar roles al propietario", "PUT", fmt.Sprintf("/api/usuarios/%d/roles", yo.ID), `{"roles":[]}`, 403},
		{"editar un rol con permisos ajenos", "PUT", fmt.Sprintf("/api/roles/%d", rolCajero), `{"nombre":"x","permisos":["productos.ver"]}`, 403},
		{"editar usuarios sin permiso", "PUT", fmt.Sprintf("/api/usuarios/%d", cajeroID), `{"nombre":"x","email":"x@example.test","estado":"inactivo"}`, 403},
	}
	for _, c := range casos {
		if got := pedirComo(t, gestor, c.metodo, srv.URL+c.ruta, c.cuerpo, nil); got != c.codigoEsperado {
			t.Errorf("gestor: %s = %d; se esperaba %d", c.nombre, got, c.codigoEsperado)
		}
	}
	if c := asignar(t, gestor, srv, cajeroID, menor.ID); c != 403 {
		t.Error("quitó al cajero un rol con permisos que el gestor no tiene", c)
	}

	// El propietario tampoco modifica sus propios roles; un rol asignado no se elimina.
	if c := asignar(t, nil, srv, yo.ID); c != 409 {
		t.Error("propietario se quitó sus roles", c)
	}
	if c := pedir(t, "DELETE", fmt.Sprintf("%s/api/roles/%d", srv.URL, rolCajero), "", nil); c != 409 {
		t.Error("eliminó un rol asignado", c)
	}
	if c := pedir(t, "POST", srv.URL+"/api/roles", fmt.Sprintf(`{"nombre":"cajero %d","permisos":[]}`, sufijo), nil); c != 409 {
		t.Error("nombre de rol duplicado sin distinguir mayúsculas", c)
	}
	if c := asignar(t, nil, srv, cajeroID); c != 200 {
		t.Fatal(c)
	}
	if c := pedirComo(t, cajero, "GET", srv.URL+"/api/productos", "", nil); c != 403 {
		t.Error("quitar el rol no revocó el acceso", c)
	}
	if c := pedir(t, "DELETE", fmt.Sprintf("%s/api/roles/%d", srv.URL, rolCajero), "", nil); c != 204 {
		t.Error("eliminar rol sin usuarios", c)
	}

	// La columna heredada rol sigue al rol Propietario (permite volver al binario anterior).
	if c := asignar(t, nil, srv, cajeroID, propietario); c != 200 {
		t.Fatal(c)
	}
	var legado string
	a.db.QueryRow(`SELECT rol FROM usuarios WHERE id=?`, cajeroID).Scan(&legado)
	if legado != "propietario" {
		t.Error("columna rol no sincronizada", legado)
	}
	if c := asignar(t, nil, srv, cajeroID); c != 200 {
		t.Fatal(c)
	}

	// Auditoría: cada cambio quedó registrado con su actor.
	var auditoria []map[string]any
	if c := pedir(t, "GET", srv.URL+"/api/auditoria?limite=500", "", &auditoria); c != 200 {
		t.Fatal("auditoría", c)
	}
	acciones := map[string]int{}
	for _, e := range auditoria {
		if fmt.Sprint(e["actor_id"]) == fmt.Sprint(yo.ID) || fmt.Sprint(e["actor_id"]) == fmt.Sprint(gestorID) {
			acciones[fmt.Sprint(e["accion"])]++
		}
	}
	for _, accion := range []string{"rol.crear", "rol.editar", "rol.eliminar", "usuario.crear", "usuario.roles"} {
		if acciones[accion] == 0 {
			t.Errorf("auditoría sin %s: %v", accion, acciones)
		}
	}
	if c := pedirComo(t, cajero, "GET", srv.URL+"/api/auditoria", "", nil); c != 403 {
		t.Error("cajero consultó la auditoría", c)
	}
}
