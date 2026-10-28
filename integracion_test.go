package main

// Pruebas de integración contra la base real. Solo corren con:
//
//	GM_INTEGRACION=1 go test -run Integracion ./...
//
// No modifican datos existentes: crean categorías de prueba y las eliminan al final.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

var clientesIntegracion sync.Map

func appIntegracion(t *testing.T) (*app, *httptest.Server) {
	t.Helper()
	if os.Getenv("GM_INTEGRACION") != "1" {
		t.Skip("prueba de integración: defina GM_INTEGRACION=1 para correrla contra MySQL")
	}
	if err := cargarEnv(".env"); err != nil {
		t.Fatal(err)
	}
	cfg, err := leerConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(cfg.DBName, "granmanzana_test") {
		t.Fatal("integración requiere DB_NAME con prefijo granmanzana_test en una base desechable")
	}
	db, err := abrirDB(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("MySQL no responde: %v", err)
	}
	a := &app{db: db}
	srv := httptest.NewServer(a.rutas())
	hash, err := bcrypt.GenerateFromPassword([]byte("ClavePruebas123!"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	email := fmt.Sprintf("prueba%d@example.test", time.Now().UnixNano())
	res, err := db.Exec(`INSERT INTO usuarios(nombre,email,password_hash,rol) VALUES('Prueba',?,?,'propietario')`, email, string(hash))
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := res.LastInsertId()
	cuerpo, _ := json.Marshal(map[string]string{"email": email, "password": "ClavePruebas123!"})
	resp, err := http.Post(srv.URL+"/api/login", "application/json", bytes.NewReader(cuerpo))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || len(resp.Cookies()) == 0 {
		t.Fatal("login integración", resp.StatusCode)
	}
	clientesIntegracion.Store(srv.URL, resp.Cookies()[0])
	t.Cleanup(func() {
		clientesIntegracion.Delete(srv.URL)
		srv.Close()
		db.Exec(`DELETE FROM usuarios WHERE id=?`, uid)
		db.Close()
	})
	return a, srv
}

func pedir(t *testing.T, metodo, url string, cuerpo string, destino any) int {
	t.Helper()
	var body *bytes.Reader
	if cuerpo != "" {
		body = bytes.NewReader([]byte(cuerpo))
	} else {
		body = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(metodo, url, body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	destinoURL, _ := neturl.Parse(url)
	if c, ok := clientesIntegracion.Load(destinoURL.Scheme + "://" + destinoURL.Host); ok {
		req.AddCookie(c.(*http.Cookie))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if destino != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(destino); err != nil {
			t.Fatalf("%s %s: JSON inválido: %v", metodo, url, err)
		}
	}
	return resp.StatusCode
}

func TestIntegracionStockYFEFO(t *testing.T) {
	a, srv := appIntegracion(t)

	var productos []productoResumen
	if c := pedir(t, "GET", srv.URL+"/api/productos", "", &productos); c != 200 || len(productos) == 0 {
		t.Fatalf("GET /api/productos = %d con %d productos", c, len(productos))
	}

	for _, p := range productos {
		// El stock del API debe coincidir con un cálculo independiente en SQL.
		var esperado sql.NullInt64
		err := a.db.QueryRow(`SELECT SUM(cantidad_actual) FROM lotes WHERE producto_id = ? AND fecha_vencimiento >= CURRENT_DATE`, p.ID).Scan(&esperado)
		if err != nil {
			t.Fatal(err)
		}
		if p.Stock != esperado.Int64 {
			t.Errorf("producto %d: stock API %d, SQL %d", p.ID, p.Stock, esperado.Int64)
		}
		if p.Stock == 0 {
			continue
		}

		// Plan FEFO por todo el stock: usa todos los lotes vendibles en orden.
		var plan struct {
			Asignacion []asignacionLote `json:"asignacion"`
		}
		url := fmt.Sprintf("%s/api/productos/%d/plan-fefo?cantidad=%d", srv.URL, p.ID, p.Stock)
		if c := pedir(t, "GET", url, "", &plan); c != 200 {
			t.Fatalf("plan FEFO producto %d = %d", p.ID, c)
		}
		var total int64
		for i, as := range plan.Asignacion {
			total += as.Tomar
			if i > 0 && plan.Asignacion[i-1].FechaVencimiento > as.FechaVencimiento {
				t.Errorf("producto %d: plan fuera de orden FEFO", p.ID)
			}
		}
		if total != p.Stock {
			t.Errorf("producto %d: el plan toma %d y el stock es %d", p.ID, total, p.Stock)
		}

		// Una unidad más que el stock debe dar 409.
		url = fmt.Sprintf("%s/api/productos/%d/plan-fefo?cantidad=%d", srv.URL, p.ID, p.Stock+1)
		if c := pedir(t, "GET", url, "", nil); c != 409 {
			t.Errorf("producto %d: pedir stock+1 = %d; se esperaba 409", p.ID, c)
		}
	}

	var maxID int64
	if err := a.db.QueryRow(`SELECT COALESCE(MAX(id), 0) FROM productos`).Scan(&maxID); err != nil {
		t.Fatal(err)
	}
	if c := pedir(t, "GET", fmt.Sprintf("%s/api/productos/%d", srv.URL, maxID+1000), "", nil); c != 404 {
		t.Errorf("producto inexistente = %d; se esperaba 404", c)
	}
}

func TestIntegracionCategoriasCicloCompleto(t *testing.T) {
	a, srv := appIntegracion(t)
	nombre := fmt.Sprintf("Prueba integración %d", time.Now().UnixNano())

	var creada categoria
	if c := pedir(t, "POST", srv.URL+"/api/categorias", `{"nombre":"`+nombre+`"}`, &creada); c != 201 {
		t.Fatalf("crear = %d", c)
	}
	// Si la prueba falla a mitad, se borra solo la categoría creada aquí.
	t.Cleanup(func() { a.db.Exec(`DELETE FROM categorias WHERE id = ? AND nombre = ?`, creada.ID, nombre) })

	if c := pedir(t, "POST", srv.URL+"/api/categorias", `{"nombre":"`+nombre+`"}`, nil); c != 409 {
		t.Errorf("duplicada = %d; se esperaba 409", c)
	}
	var leida categoria
	if c := pedir(t, "GET", fmt.Sprintf("%s/api/categorias/%d", srv.URL, creada.ID), "", &leida); c != 200 || leida.Nombre != nombre {
		t.Errorf("consultar = %d %+v", c, leida)
	}

	var enUso int64
	if err := a.db.QueryRow(`SELECT categoria_id FROM productos LIMIT 1`).Scan(&enUso); err != nil {
		t.Fatal(err)
	}
	if c := pedir(t, "DELETE", fmt.Sprintf("%s/api/categorias/%d", srv.URL, enUso), "", nil); c != 409 {
		t.Errorf("eliminar categoría con productos = %d; se esperaba 409", c)
	}

	if c := pedir(t, "DELETE", fmt.Sprintf("%s/api/categorias/%d", srv.URL, creada.ID), "", nil); c != 204 {
		t.Errorf("eliminar = %d; se esperaba 204", c)
	}
	if c := pedir(t, "DELETE", fmt.Sprintf("%s/api/categorias/%d", srv.URL, creada.ID), "", nil); c != 404 {
		t.Errorf("eliminar dos veces = %d; se esperaba 404", c)
	}
}

func TestIntegracionRecomendacionesEvitanContraindicaciones(t *testing.T) {
	a, srv := appIntegracion(t)

	var necesidades, contras []etiquetaConConteo
	pedir(t, "GET", srv.URL+"/api/etiquetas?tipo=necesidad", "", &necesidades)
	pedir(t, "GET", srv.URL+"/api/etiquetas?tipo=contraindicacion", "", &contras)
	if len(necesidades) == 0 || len(contras) == 0 {
		t.Skip("no hay etiquetas suficientes para la prueba")
	}

	for _, n := range necesidades {
		for _, c := range contras {
			var r struct {
				Productos []productoResumen `json:"productos"`
			}
			url := fmt.Sprintf("%s/api/recomendaciones?necesidad=%d&evitar=%d", srv.URL, n.ID, c.ID)
			if code := pedir(t, "GET", url, "", &r); code != 200 {
				t.Fatalf("%s = %d", url, code)
			}
			for _, p := range r.Productos {
				var tiene int
				a.db.QueryRow(`SELECT COUNT(*) FROM producto_etiqueta WHERE producto_id = ? AND etiqueta_id = ?`, p.ID, c.ID).Scan(&tiene)
				if tiene > 0 {
					t.Errorf("necesidad %d evitando %d devolvió el producto %d, que tiene esa contraindicación", n.ID, c.ID, p.ID)
				}
			}
		}
	}

	if code := pedir(t, "GET", fmt.Sprintf("%s/api/recomendaciones?necesidad=%d", srv.URL, contras[0].ID), "", nil); code != 400 {
		t.Errorf("necesidad con etiqueta de contraindicación = %d; se esperaba 400", code)
	}
}

func TestIntegracionAlertas(t *testing.T) {
	_, srv := appIntegracion(t)

	var venc []alertaVencimiento
	if c := pedir(t, "GET", srv.URL+"/api/alertas/vencimientos", "", &venc); c != 200 {
		t.Fatalf("vencimientos = %d", c)
	}
	for _, v := range venc {
		if v.Nivel != clasificarVencimiento(int(v.DiasRestantes), int(v.DiasAlertaLiquidacion)) {
			t.Errorf("lote %d: nivel %q inconsistente", v.LoteID, v.Nivel)
		}
		if v.AccionSugerida == "" || v.CantidadActual <= 0 {
			t.Errorf("lote %d: alerta incompleta %+v", v.LoteID, v)
		}
	}

	var bajos []alertaStock
	if c := pedir(t, "GET", srv.URL+"/api/alertas/stock-bajo", "", &bajos); c != 200 {
		t.Fatalf("stock-bajo = %d", c)
	}
	for _, b := range bajos {
		if b.Stock > b.StockMinimo || b.FaltanteParaMinimo != max(b.StockMinimo-b.Stock, 0) {
			t.Errorf("producto %d: alerta de stock inconsistente %+v", b.ID, b)
		}
	}
}
