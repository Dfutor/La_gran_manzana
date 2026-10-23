// Comando granmanzana expone el API de inventario por lotes (FEFO) y de
// consulta/recomendación de productos de la tienda naturista «La Gran Manzana»,
// junto con un mini frontend de prueba en /granmanzana/.
package main

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"
)

//go:embed web
var webFS embed.FS

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

	a := &app{db: db}
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
	mux.HandleFunc("GET /api/productos", a.listarProductos)
	mux.HandleFunc("GET /api/productos/{id}", a.detalleProducto)
	mux.HandleFunc("GET /api/etiquetas", a.listarEtiquetas)
	mux.HandleFunc("GET /api/etiquetas/{id}/productos", a.productosPorEtiqueta)
	mux.HandleFunc("GET /api/alertas/stock-bajo", a.alertasStockBajo)
	mux.HandleFunc("GET /api/alertas/vencimientos", a.alertasVencimientos)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			w.Header().Set("Allow", "GET, POST, OPTIONS")
			escribirError(w, http.StatusMethodNotAllowed, "método no permitido: "+r.Method)
			return
		}
		escribirError(w, http.StatusNotFound, "ruta no encontrada: "+r.Method+" "+r.URL.Path)
	})

	web, err := fs.Sub(webFS, "web")
	if err != nil {
		panic(err)
	}
	archivos := http.StripPrefix("/granmanzana", http.FileServerFS(web))
	mux.Handle("GET /granmanzana/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		archivos.ServeHTTP(w, r)
	}))
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

func conLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inicio := time.Now()
		rec := &registroEstado{ResponseWriter: w, codigo: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.RequestURI(), rec.codigo, time.Since(inicio).Round(time.Microsecond))
	})
}

func conCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Content-Type")
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
