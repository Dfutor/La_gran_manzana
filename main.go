// Comando granmanzana expone el API de inventario por lotes (FEFO) y de
// consulta/recomendación de productos de la tienda naturista «La Gran Manzana»,
// junto con un mini frontend de prueba en /granmanzana/.
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"errors"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"
)

//go:embed web
var webFS embed.FS

//go:embed docs_web
var docsFS embed.FS

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	crearPropietario := flag.String("crear-propietario", "", "crear el primer propietario; contraseña por stdin")
	restablecer := flag.String("restablecer-clave", "", "restablecer contraseña por email; contraseña por stdin")
	verificar := flag.Bool("verificar", false, "solo comprobar conexión y tablas (antes de reemplazar el binario en producción)")
	flag.Parse()

	if err := cargarEnv(".env"); err != nil {
		log.Fatalf("no se pudo leer .env: %v", err)
	}
	cfg, err := leerConfig()
	if err != nil {
		log.Fatalf("configuración incompleta: %v", err)
	}

	db, err := abrirDB(cfg)
	if err != nil {
		log.Fatalf("configuración de MySQL inválida: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err = db.PingContext(ctx)
	cancel()
	if err != nil {
		log.Fatalf("no hay conexión con MySQL en %s (base %s): %v", cfg.DBHost, cfg.DBName, err)
	}
	log.Printf("conectado a MySQL %s, base %s", cfg.DBHost, cfg.DBName)

	if *verificar {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := verificarEsquema(ctx, db); err != nil {
			log.Fatalf("verificación fallida: %v", err)
		}
		log.Printf("verificación correcta: conexión y tablas listas; puede reemplazar el binario")
		return
	}

	ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	err = iniciarRBAC(ctx, db)
	cancel()
	if err != nil {
		log.Fatalf("control de acceso: %v", err)
	}

	if *crearPropietario != "" || *restablecer != "" {
		if err := configurarAcceso(db, *crearPropietario, *restablecer); err != nil {
			log.Fatal(err)
		}
		return
	}
	a := &app{db: db, docsUsuario: cfg.DocsUser, docsClave: cfg.DocsPass, cookieSegura: cfg.CookieSegura}
	alertasCtx, detenerAlertas := context.WithCancel(context.Background())
	defer detenerAlertas()
	go a.vigilarAlertas(alertasCtx)
	if cfg.DocsUser == "" || cfg.DocsPass == "" {
		log.Printf("aviso: DOCS_USER/DOCS_PASS vacíos; la documentación privada queda deshabilitada")
	}
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           a.rutas(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Apagado ordenado cuando systemd envía SIGTERM.
	errCh := make(chan error, 1)
	go func() {
		log.Printf("API escuchando en http://%s", cfg.Addr)
		errCh <- srv.ListenAndServe()
	}()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("el servidor se detuvo: %v", err)
		}
	case s := <-sig:
		log.Printf("señal %v recibida, cerrando…", s)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("cierre forzado: %v", err)
		}
	}
}

// rutas arma el enrutador con todos los endpoints, el frontend y los middlewares.
func (a *app) rutas() http.Handler {
	if a.auth == nil {
		a.auth = nuevaAutenticacion()
	}
	return conRecuperacion(conLog(conCORS(a.protegerAPI(a.enrutador()))))
}

// rutaAPI asocia cada endpoint con el permiso que exige. protegerAPI niega
// cualquier ruta /api/ que no aparezca aquí.
type rutaAPI struct {
	patron  string
	permiso string // accesoPublico, accesoSesion o un permiso del catálogo
	manejar http.HandlerFunc
}

func (a *app) rutasAPI() []rutaAPI {
	return []rutaAPI{
		{"GET /api/salud", accesoPublico, a.salud},
		{"POST /api/login", accesoPublico, a.login},
		{"POST /api/logout", accesoPublico, a.logout},
		{"GET /api/sesion", accesoSesion, a.sesionActual},

		{"GET /api/usuarios", "usuarios.ver", a.listarUsuarios},
		{"POST /api/usuarios", "usuarios.crear", a.crearUsuario},
		{"PUT /api/usuarios/{id}", "usuarios.editar", a.actualizarUsuario},
		{"GET /api/usuarios/{id}/permisos", "usuarios.ver", a.permisosUsuario},
		{"PUT /api/usuarios/{id}/roles", "roles.asignar", a.asignarRolesUsuario},
		{"PUT /api/usuarios/{id}/permisos", "roles.asignar", a.asignarPermisosUsuario},
		{"GET /api/permisos", "roles.ver", a.listarPermisos},
		{"GET /api/roles", "roles.ver", a.listarRoles},
		{"GET /api/roles/{id}", "roles.ver", a.obtenerRol},
		{"POST /api/roles", "roles.gestionar", a.crearRol},
		{"PUT /api/roles/{id}", "roles.gestionar", a.actualizarRol},
		{"DELETE /api/roles/{id}", "roles.gestionar", a.eliminarRol},
		{"GET /api/auditoria", "auditoria.ver", a.listarAuditoria},

		{"GET /api/lotes", "lotes.ver", a.listarLotes},
		{"POST /api/lotes", "lotes.crear", a.crearLote},
		{"PUT /api/lotes/{id}", "lotes.editar", a.actualizarLote},
		{"GET /api/ventas", "ventas.ver", a.listarVentas},
		{"GET /api/ventas/{id}", "ventas.ver", a.detalleVenta},
		{"POST /api/ventas", "ventas.crear", a.crearVenta},
		{"GET /api/bajas", "bajas.ver", a.listarBajas},
		{"POST /api/bajas", "bajas.crear", a.crearBaja},
		{"GET /api/reportes/ventas", "reportes.ventas", a.reporteVentas},
		{"GET /api/reportes/perdidas", "reportes.perdidas", a.reportePerdidas},
		{"GET /api/reportes/rotacion", "reportes.rotacion", a.reporteRotacion},

		{"GET /api/categorias", "categorias.ver", a.listarCategorias},
		{"POST /api/categorias", "categorias.crear", a.crearCategoria},
		{"PUT /api/categorias/{id}", "categorias.editar", a.actualizarCategoria},
		{"GET /api/categorias/{id}", "categorias.ver", a.obtenerCategoria},
		{"DELETE /api/categorias/{id}", "categorias.eliminar", a.eliminarCategoria},
		{"GET /api/proveedores", "proveedores.ver", a.listarProveedores},
		{"POST /api/proveedores", "proveedores.crear", a.crearProveedor},
		{"PUT /api/proveedores/{id}", "proveedores.editar", a.actualizarProveedor},
		{"DELETE /api/proveedores/{id}", "proveedores.eliminar", a.eliminarProveedor},
		{"GET /api/productos", "productos.ver", a.listarProductos},
		{"POST /api/productos", "productos.crear", a.crearProducto},
		{"PUT /api/productos/{id}", "productos.editar", a.actualizarProducto},
		{"DELETE /api/productos/{id}", "productos.eliminar", a.eliminarProducto},
		{"GET /api/productos/{id}", "productos.ver", a.detalleProducto},
		{"GET /api/productos/{id}/plan-fefo", "productos.ver", a.planFEFO},
		{"GET /api/etiquetas", "etiquetas.ver", a.listarEtiquetas},
		{"POST /api/etiquetas", "etiquetas.crear", a.crearEtiqueta},
		{"PUT /api/etiquetas/{id}", "etiquetas.editar", a.actualizarEtiqueta},
		{"DELETE /api/etiquetas/{id}", "etiquetas.eliminar", a.eliminarEtiqueta},
		{"GET /api/etiquetas/{id}/productos", "etiquetas.ver", a.productosPorEtiqueta},
		{"GET /api/recomendaciones", "recomendaciones.consultar", a.recomendaciones},
		{"GET /api/alertas/stock-bajo", "alertas.ver", a.alertasStockBajo},
		{"GET /api/alertas/vencimientos", "alertas.ver", a.alertasVencimientos},
		{"GET /api/sugerencias", "alertas.ver", a.listarSugerencias},
		{"POST /api/sugerencias/decision", "alertas.decidir", a.decidirSugerencia},

		// Respuesta 404/405 para el resto de /api/ (requiere sesión, no revela rutas).
		{"/api/", accesoSesion, rutaNoEncontrada},
	}
}

var permisoDeRuta = func() map[string]string {
	m := map[string]string{}
	for _, r := range (&app{}).rutasAPI() {
		m[r.patron] = r.permiso
	}
	return m
}()

func rutaNoEncontrada(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodPost, http.MethodDelete:
		escribirError(w, http.StatusNotFound, "ruta no encontrada: "+r.Method+" "+r.URL.Path)
	default:
		w.Header().Set("Allow", "GET, POST, PUT, DELETE, OPTIONS")
		escribirError(w, http.StatusMethodNotAllowed, "método no permitido: "+r.Method)
	}
}

func (a *app) enrutador() *http.ServeMux {
	mux := http.NewServeMux()
	for _, r := range a.rutasAPI() {
		mux.HandleFunc(r.patron, r.manejar)
	}

	// Frontend público de prueba.
	web, err := fs.Sub(webFS, "web")
	if err != nil {
		panic(err)
	}
	archivos := http.StripPrefix("/granmanzana", http.FileServerFS(web))
	indice, err := fs.ReadFile(web, "index.html")
	if err != nil {
		panic(err)
	}
	mux.Handle("GET /granmanzana/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Frame-Options", "DENY")
		// Rutas de la aplicación (/granmanzana/login, /granmanzana/admin/usuarios, …):
		// sin extensión y sin archivo propio, se responden con index.html y las
		// resuelve el enrutador del navegador. Los recursos inexistentes siguen dando 404.
		ruta := strings.TrimPrefix(r.URL.Path, "/granmanzana/")
		if path.Ext(ruta) == "" {
			if info, err := fs.Stat(web, strings.TrimSuffix(ruta, "/")); ruta == "" || err != nil || info.IsDir() {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Write(indice)
				return
			}
		}
		archivos.ServeHTTP(w, r)
	}))

	// Documentación privada (usuario y clave en .env).
	docs, err := fs.Sub(docsFS, "docs_web")
	if err != nil {
		panic(err)
	}
	mux.Handle("GET /granmanzana/docs/", a.conClaveDocs(http.StripPrefix("/granmanzana/docs", http.FileServerFS(docs))))
	mux.HandleFunc("GET /granmanzana/docs", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/granmanzana/docs/", http.StatusMovedPermanently)
	})

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/granmanzana/", http.StatusFound)
	})

	return mux
}

// --- Middlewares ---

type registroEstado struct {
	http.ResponseWriter
	codigo int
}

func (r *registroEstado) WriteHeader(codigo int) {
	r.codigo = codigo
	r.ResponseWriter.WriteHeader(codigo)
}

type claveCtx struct{}

// idPeticion devuelve el identificador corto que conLog asigna a cada petición.
func idPeticion(ctx context.Context) string {
	if id, ok := ctx.Value(claveCtx{}).(string); ok {
		return id
	}
	return "-"
}

func nuevoIDPeticion() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b)
}

// conLog asigna un X-Request-Id a cada petición y registra método, ruta,
// código y duración.
func conLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inicio := time.Now()
		id := nuevoIDPeticion()
		w.Header().Set("X-Request-Id", id)
		r = r.WithContext(context.WithValue(r.Context(), claveCtx{}, id))
		rec := &registroEstado{ResponseWriter: w, codigo: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("[%s] %s %s %d %s", id, r.Method, r.URL.EscapedPath(), rec.codigo, time.Since(inicio).Round(time.Microsecond))
	})
}

func conCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		h.Set("Access-Control-Expose-Headers", "X-Request-Id, Location")
		h.Set("X-Content-Type-Options", "nosniff")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func conRecuperacion(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				if p == http.ErrAbortHandler {
					panic(p)
				}
				log.Printf("PANIC en %s %s: %v\n%s", r.Method, r.URL.Path, p, debug.Stack())
				escribirError(w, http.StatusInternalServerError, "error interno del servidor")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// conClaveDocs protege la documentación con autenticación HTTP Basic.
// Compara hashes en tiempo constante para no filtrar información por tiempos.
func (a *app) conClaveDocs(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		if a.docsUsuario == "" || a.docsClave == "" {
			http.Error(w, "Documentación deshabilitada: falta DOCS_USER/DOCS_PASS en .env", http.StatusServiceUnavailable)
			return
		}
		usuario, clave, ok := r.BasicAuth()
		if ok && iguales(usuario, a.docsUsuario) && iguales(clave, a.docsClave) {
			if strings.HasSuffix(r.URL.Path, ".json") {
				w.Header().Set("Content-Disposition", `attachment; filename="`+path.Base(r.URL.Path)+`"`)
			}
			next.ServeHTTP(w, r)
			return
		}
		if ok {
			log.Printf("[%s] docs: credenciales incorrectas para el usuario %q", idPeticion(r.Context()), usuario)
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="Documentacion La Gran Manzana", charset="UTF-8"`)
		http.Error(w, "Se requiere usuario y contraseña", http.StatusUnauthorized)
	})
}

func iguales(a, b string) bool {
	ha, hb := sha256.Sum256([]byte(a)), sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}
