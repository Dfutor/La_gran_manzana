package main

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestIntegracionOverrides(t *testing.T) {
	a, srv := appIntegracion(t)
	sufijo := time.Now().UnixNano()
	var yo perfilUsuario
	pedir(t, "GET", srv.URL+"/api/sesion", "", &yo)

	cajeroID, cajeroEmail := crearUsuarioPrueba(t, a, srv, "override")
	rol := crearRolPrueba(t, a, srv, fmt.Sprintf("Caja %d", sufijo), "productos.ver", "ventas.ver", "ventas.crear")
	if c := asignar(t, nil, srv, cajeroID, rol); c != 200 {
		t.Fatal(c)
	}
	cajero := iniciarSesion(t, srv, cajeroEmail, "ClaveRBAC12345!")
	personalizar := func(overrides string) int {
		return pedir(t, "PUT", fmt.Sprintf("%s/api/usuarios/%d/permisos", srv.URL, cajeroID), `{"overrides":`+overrides+`}`, nil)
	}

	// Override > rol: se deniega algo que el rol da y se concede algo que no da.
	if c := personalizar(`[{"permiso":"productos.ver","concedido":false},{"permiso":"reportes.ventas","concedido":true}]`); c != 200 {
		t.Fatal("personalizar", c)
	}
	if c := pedirComo(t, cajero, "GET", srv.URL+"/api/productos", "", nil); c != 403 {
		t.Error("la denegación no prevaleció sobre el rol", c)
	}
	if c := pedirComo(t, cajero, "GET", srv.URL+"/api/reportes/ventas", "", nil); c != 200 {
		t.Error("la concesión no aplicó", c)
	}
	var detalle struct {
		Permisos  []permisoEfectivo `json:"permisos"`
		Detalle   []permisoDetalle  `json:"detalle"`
		Overrides []map[string]any  `json:"overrides"`
	}
	pedir(t, "GET", fmt.Sprintf("%s/api/usuarios/%d/permisos", srv.URL, cajeroID), "", &detalle)
	porNombre := map[string]permisoDetalle{}
	for _, d := range detalle.Detalle {
		porNombre[d.Nombre] = d
	}
	if d := porNombre["productos.ver"]; !d.Heredado || d.Override != "denegado" || d.Efectivo {
		t.Errorf("productos.ver: %+v", d)
	}
	if d := porNombre["reportes.ventas"]; d.Heredado || d.Override != "concedido" || !d.Efectivo {
		t.Errorf("reportes.ventas: %+v", d)
	}
	origen := map[string]string{}
	for _, p := range detalle.Permisos {
		origen[p.Nombre] = p.Origen
	}
	if origen["reportes.ventas"] != "personalizado" || origen["ventas.crear"] != "rol" || origen["productos.ver"] != "" || len(detalle.Overrides) != 2 {
		t.Errorf("efectivos: %v overrides=%v", origen, detalle.Overrides)
	}

	// Restablecer al rol.
	if c := personalizar(`[]`); c != 200 {
		t.Fatal(c)
	}
	if c := pedirComo(t, cajero, "GET", srv.URL+"/api/productos", "", nil); c != 200 {
		t.Error("restablecer no devolvió el permiso del rol", c)
	}
	if c := pedirComo(t, cajero, "GET", srv.URL+"/api/reportes/ventas", "", nil); c != 403 {
		t.Error("restablecer no quitó la concesión", c)
	}

	// Reglas: no a uno mismo, no denegar al Propietario, validaciones.
	if c := pedir(t, "PUT", fmt.Sprintf("%s/api/usuarios/%d/permisos", srv.URL, yo.ID), `{"overrides":[]}`, nil); c != 409 {
		t.Error("se personalizó a sí mismo", c)
	}
	propID, _ := crearUsuarioPrueba(t, a, srv, "otroprop")
	if c := asignar(t, nil, srv, propID, rolPorNombre(t, srv, nombreRolPropietario)); c != 200 {
		t.Fatal(c)
	}
	if c := pedir(t, "PUT", fmt.Sprintf("%s/api/usuarios/%d/permisos", srv.URL, propID), `{"overrides":[{"permiso":"usuarios.ver","concedido":false}]}`, nil); c != 409 {
		t.Error("denegó un permiso a un Propietario", c)
	}
	for _, cuerpo := range []string{`{}`, `{"overrides":[{"permiso":"todo.hacer","concedido":true}]}`, `{"overrides":[{"permiso":"productos.ver"}]}`} {
		if c := pedir(t, "PUT", fmt.Sprintf("%s/api/usuarios/%d/permisos", srv.URL, cajeroID), cuerpo, nil); c != 400 {
			t.Errorf("%s = %d", cuerpo, c)
		}
	}

	// Un administrador delegado no concede lo que no tiene ni toca a quien tiene más.
	gestorRol := crearRolPrueba(t, a, srv, fmt.Sprintf("Delegado %d", sufijo), "roles.asignar", "usuarios.ver", "productos.ver", "ventas.ver", "ventas.crear")
	gestorID, gestorEmail := crearUsuarioPrueba(t, a, srv, "delegado")
	asignar(t, nil, srv, gestorID, gestorRol)
	gestor := iniciarSesion(t, srv, gestorEmail, "ClaveRBAC12345!")
	if c := pedirComo(t, gestor, "PUT", fmt.Sprintf("%s/api/usuarios/%d/permisos", srv.URL, cajeroID), `{"overrides":[{"permiso":"reportes.ventas","concedido":true}]}`, nil); c != 403 {
		t.Error("concedió un permiso que no tiene", c)
	}
	if c := pedirComo(t, gestor, "PUT", fmt.Sprintf("%s/api/usuarios/%d/permisos", srv.URL, cajeroID), `{"overrides":[{"permiso":"ventas.crear","concedido":false}]}`, nil); c != 200 {
		t.Error("no pudo personalizar un permiso que sí tiene", c)
	}
	if c := pedirComo(t, gestor, "PUT", fmt.Sprintf("%s/api/usuarios/%d/permisos", srv.URL, propID), `{"overrides":[]}`, nil); c != 403 {
		t.Error("modificó a un usuario con más permisos", c)
	}

	var auditoria []map[string]any
	pedir(t, "GET", srv.URL+"/api/auditoria?limite=100", "", &auditoria)
	encontrada := false
	for _, e := range auditoria {
		if e["accion"] == "usuario.permisos" && fmt.Sprint(e["entidad_id"]) == fmt.Sprint(cajeroID) {
			encontrada = true
		}
	}
	if !encontrada {
		t.Error("la personalización no quedó en la auditoría")
	}
}

func TestIntegracionSugerencias(t *testing.T) {
	a, srv := appIntegracion(t)
	// Un producto sin lotes queda en stock bajo (0 <= mínimo) y genera «reponer».
	producto := productoPrueba(t, a, srv)
	clave := fmt.Sprintf("stock:%d:0", producto)
	t.Cleanup(func() { a.db.Exec(`DELETE FROM sugerencias_decisiones WHERE clave=?`, clave) })
	buscar := func() *sugerencia {
		var l []sugerencia
		if c := pedir(t, "GET", srv.URL+"/api/sugerencias", "", &l); c != 200 {
			t.Fatal("GET sugerencias", c)
		}
		for i := range l {
			if l[i].Clave == clave {
				return &l[i]
			}
		}
		return nil
	}
	s := buscar()
	if s == nil || s.Tipo != "reponer" || s.Destino != "lote" || s.Prioridad != 1 {
		t.Fatalf("no apareció la recomendación de reponer: %+v", s)
	}

	usuarioID, email := crearUsuarioPrueba(t, a, srv, "sinDecidir")
	rol := crearRolPrueba(t, a, srv, fmt.Sprintf("Solo alertas %d", time.Now().UnixNano()), "alertas.ver")
	asignar(t, nil, srv, usuarioID, rol)
	consulta := iniciarSesion(t, srv, email, "ClaveRBAC12345!")
	cuerpo, _ := json.Marshal(map[string]string{"clave": clave, "decision": "descartada"})
	if c := pedirComo(t, consulta, "POST", srv.URL+"/api/sugerencias/decision", string(cuerpo), nil); c != 403 {
		t.Error("decidió sin alertas.decidir", c)
	}
	if c := pedir(t, "POST", srv.URL+"/api/sugerencias/decision", string(cuerpo), nil); c != 200 {
		t.Fatal("descartar", c)
	}
	if buscar() != nil {
		t.Error("la recomendación descartada sigue apareciendo")
	}
	if c := pedir(t, "POST", srv.URL+"/api/sugerencias/decision", string(cuerpo), nil); c != 409 {
		t.Error("decisión repetida", c)
	}
	if c := pedir(t, "POST", srv.URL+"/api/sugerencias/decision", `{"clave":"stock:999999999:0","decision":"aceptada"}`, nil); c != 404 {
		t.Error("clave inexistente", c)
	}
	if c := pedir(t, "POST", srv.URL+"/api/sugerencias/decision", `{"clave":"x","decision":"quizas"}`, nil); c != 400 {
		t.Error("decisión inválida", c)
	}

	// Si la situación cambia (ingresa un lote), la clave cambia y vuelve a evaluarse.
	ingresarPrueba(t, srv, producto, 400, 1)
	if s := buscar(); s != nil {
		t.Errorf("con stock 1 y mínimo 0 no debería pedir reponer: %+v", s)
	}
}
