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

	a := &app{db: db, docsUsuario: cfg.DocsUser, docsClave: cfg.DocsPass}
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
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/salud", a.salud)
	mux.HandleFunc("GET /api/categorias", a.listarCategorias)
	mux.HandleFunc("POST /api/categorias", a.crearCategoria)
	mux.HandleFunc("PUT /api/categorias/{id}", a.actualizarCategoria)
	mux.HandleFunc("GET /api/categorias/{id}", a.obtenerCategoria)
	mux.HandleFunc("DELETE /api/categorias/{id}", a.eliminarCategoria)
	mux.HandleFunc("GET /api/proveedores", a.listarProveedores)
	mux.HandleFunc("POST /api/proveedores", a.crearProveedor)
	mux.HandleFunc("PUT /api/proveedores/{id}", a.actualizarProveedor)
	mux.HandleFunc("DELETE /api/proveedores/{id}", a.eliminarProveedor)
	mux.HandleFunc("GET /api/productos", a.listarProductos)
	mux.HandleFunc("POST /api/productos", a.crearProducto)
	mux.HandleFunc("PUT /api/productos/{id}", a.actualizarProducto)
	mux.HandleFunc("DELETE /api/productos/{id}", a.eliminarProducto)
	mux.HandleFunc("GET /api/productos/{id}", a.detalleProducto)
	mux.HandleFunc("GET /api/productos/{id}/plan-fefo", a.planFEFO)
	mux.HandleFunc("GET /api/etiquetas", a.listarEtiquetas)
	mux.HandleFunc("POST /api/etiquetas", a.crearEtiqueta)
	mux.HandleFunc("PUT /api/etiquetas/{id}", a.actualizarEtiqueta)
	mux.HandleFunc("DELETE /api/etiquetas/{id}", a.eliminarEtiqueta)
	mux.HandleFunc("GET /api/etiquetas/{id}/productos", a.productosPorEtiqueta)
	mux.HandleFunc("GET /api/recomendaciones", a.recomendaciones)
	mux.HandleFunc("GET /api/alertas/stock-bajo", a.alertasStockBajo)
	mux.HandleFunc("GET /api/alertas/vencimientos", a.alertasVencimientos)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodPost, http.MethodDelete:
			escribirError(w, http.StatusNotFound, "ruta no encontrada: "+r.Method+" "+r.URL.Path)
		default:
			w.Header().Set("Allow", "GET, POST, PUT, DELETE, OPTIONS")
			escribirError(w, http.StatusMethodNotAllowed, "método no permitido: "+r.Method)
		}
	})

	// Frontend público de prueba.
	web, err := fs.Sub(webFS, "web")
	if err != nil {
		panic(err)
	}
	archivos := http.StripPrefix("/granmanzana", http.FileServerFS(web))
	mux.Handle("GET /granmanzana/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
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

	return conRecuperacion(conLog(conCORS(mux)))
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
		log.Printf("[%s] %s %s %d %s", id, r.Method, r.URL.RequestURI(), rec.codigo, time.Since(inicio).Round(time.Microsecond))
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
