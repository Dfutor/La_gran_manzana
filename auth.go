package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const nombreCookie = "granmanzana_sesion"
const duracionSesion = 8 * time.Hour

type usuario struct {
	ID     int64  `json:"id"`
	Nombre string `json:"nombre"`
	Email  string `json:"email"`
	Rol    string `json:"rol"`
	Estado string `json:"estado"`
}
type sesion struct {
	UsuarioID int64
	Hash      string
	Vence     time.Time
}
type intentoLogin struct {
	Cuenta int
	Vence  time.Time
}
type autenticacion struct {
	mu        sync.Mutex
	sesiones  map[[32]byte]sesion
	intentos  map[string]intentoLogin
	hashFalso []byte
}
type usuarioCtx struct{}

func nuevaAutenticacion() *autenticacion {
	falso, err := bcrypt.GenerateFromPassword([]byte(nuevoIDPeticion()), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	return &autenticacion{sesiones: make(map[[32]byte]sesion), intentos: make(map[string]intentoLogin), hashFalso: falso}
}
func usuarioActual(r *http.Request) usuario {
	u, _ := r.Context().Value(usuarioCtx{}).(usuario)
	return u
}
func emailValido(s string) bool {
	e, err := mail.ParseAddress(s)
	return err == nil && e.Address == s && len(s) <= 150
}
func validarClave(s string) bool { return len(s) >= 12 && len(s) <= 72 }

// Se reserva el intento antes de bcrypt para limitar también peticiones simultáneas.
func (a *autenticacion) permitirLogin(ip string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	ahora := time.Now()
	for k, v := range a.intentos {
		if ahora.After(v.Vence) {
			delete(a.intentos, k)
		}
	}
	v := a.intentos[ip]
	if v.Cuenta >= 10 {
		return false
	}
	if v.Cuenta == 0 {
		if len(a.intentos) >= 10000 {
			return false
		}
		v.Vence = ahora.Add(15 * time.Minute)
	}
	v.Cuenta++
	a.intentos[ip] = v
	return true
}
func (a *app) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodificarJSON(w, r, &in) {
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if !emailValido(in.Email) || len(in.Password) == 0 || len(in.Password) > 72 {
		escribirError(w, 400, "correo o contraseña inválidos")
		return
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	if !a.auth.permitirLogin(ip) {
		w.Header().Set("Retry-After", "900")
		escribirError(w, 429, "demasiados intentos; espere 15 minutos")
		return
	}
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	var u usuario
	var hash string
	err = a.db.QueryRowContext(ctx, `SELECT id,nombre,email,rol,estado,password_hash FROM usuarios WHERE email=?`, in.Email).Scan(&u.ID, &u.Nombre, &u.Email, &u.Rol, &u.Estado, &hash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		errorInterno(w, r, err)
		return
	}
	comparacion := []byte(hash)
	if err != nil || !strings.HasPrefix(hash, "$2") {
		comparacion = a.auth.hashFalso
	}
	claveErr := bcrypt.CompareHashAndPassword(comparacion, []byte(in.Password))
	if err != nil || claveErr != nil || u.Estado != "activo" || !strings.HasPrefix(hash, "$2") {
		escribirError(w, 401, "correo o contraseña incorrectos")
		return
	}
	if _, err = a.db.ExecContext(ctx, `UPDATE usuarios SET fecha_ultimo_login=NOW() WHERE id=?`, u.ID); err != nil {
		errorInterno(w, r, err)
		return
	}
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		errorInterno(w, r, err)
		return
	}
	token := hex.EncodeToString(b)
	a.auth.mu.Lock()
	for k, v := range a.auth.sesiones {
		if time.Now().After(v.Vence) {
			delete(a.auth.sesiones, k)
		}
	}
	if len(a.auth.sesiones) >= 10000 {
		a.auth.mu.Unlock()
		escribirError(w, 503, "límite de sesiones; intente más tarde")
		return
	}
	a.auth.sesiones[sha256.Sum256([]byte(token))] = sesion{u.ID, hash, time.Now().Add(duracionSesion)}
	delete(a.auth.intentos, ip)
	a.auth.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: nombreCookie, Value: token, Path: "/api/", HttpOnly: true, Secure: a.cookieSegura, SameSite: http.SameSiteStrictMode, MaxAge: int(duracionSesion.Seconds())})
	escribirJSON(w, 200, u)
}
func (a *app) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(nombreCookie); err == nil {
		a.auth.mu.Lock()
		delete(a.auth.sesiones, sha256.Sum256([]byte(c.Value)))
		a.auth.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: nombreCookie, Path: "/api/", Value: "", HttpOnly: true, Secure: a.cookieSegura, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	w.WriteHeader(204)
}
func (a *app) sesionActual(w http.ResponseWriter, r *http.Request) {
	escribirJSON(w, 200, usuarioActual(r))
}

func requierePropietario(r *http.Request) bool {
	p := r.URL.Path
	if strings.HasPrefix(p, "/api/usuarios") || strings.HasPrefix(p, "/api/reportes/") {
		return true
	}
	if r.Method == "GET" || r.Method == "HEAD" {
		return false
	}
	return p != "/api/ventas" && p != "/api/bajas" && p != "/api/lotes" && p != "/api/logout"
}

// Apache puede conservar Host o enviar X-Forwarded-Host. Solo se confía en
// ese encabezado cuando la conexión llega desde el proxy local.
func hostPublico(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && net.ParseIP(ip).IsLoopback() {
		if forwarded := r.Header.Get("X-Forwarded-Host"); forwarded != "" {
			hosts := strings.Split(forwarded, ",")
			return strings.TrimSpace(hosts[len(hosts)-1])
		}
	}
	return r.Host
}

func (a *app) protegerAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
			if origen := r.Header.Get("Origin"); origen != "" {
				u, err := url.Parse(origen)
				if err != nil || !strings.EqualFold(u.Host, hostPublico(r)) || (u.Scheme != "http" && u.Scheme != "https") {
					escribirError(w, 403, "origen no permitido")
					return
				}
			}
			if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				escribirError(w, 403, "origen no permitido")
				return
			}
		}
		if r.URL.Path == "/api/salud" || r.URL.Path == "/api/login" || r.URL.Path == "/api/logout" {
			next.ServeHTTP(w, r)
			return
		}
		c, err := r.Cookie(nombreCookie)
		if err != nil {
			escribirError(w, 401, "inicie sesión")
			return
		}
		key := sha256.Sum256([]byte(c.Value))
		a.auth.mu.Lock()
		s, ok := a.auth.sesiones[key]
		if ok && time.Now().After(s.Vence) {
			delete(a.auth.sesiones, key)
			ok = false
		}
		a.auth.mu.Unlock()
		if !ok {
			escribirError(w, 401, "sesión vencida; inicie sesión")
			return
		}
		ctx, cancel := ctxConsulta(r)
		defer cancel()
		var u usuario
		var hash string
		err = a.db.QueryRowContext(ctx, `SELECT id,nombre,email,rol,estado,password_hash FROM usuarios WHERE id=?`, s.UsuarioID).Scan(&u.ID, &u.Nombre, &u.Email, &u.Rol, &u.Estado, &hash)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			errorInterno(w, r, err)
			return
		}
		if err != nil || u.Estado != "activo" || hash != s.Hash {
			escribirError(w, 401, "sesión inválida; inicie sesión")
			return
		}
		if requierePropietario(r) && u.Rol != "propietario" {
			escribirError(w, 403, "se requiere rol propietario")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), usuarioCtx{}, u)))
	})
}

type usuarioEntrada struct {
	Nombre   string `json:"nombre"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Rol      string `json:"rol"`
	Estado   string `json:"estado"`
}

func validarUsuario(in *usuarioEntrada, nuevo bool) bool {
	in.Nombre = strings.TrimSpace(in.Nombre)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if in.Estado == "" {
		in.Estado = "activo"
	}
	if in.Rol == "" {
		in.Rol = "empleado"
	}
	return in.Nombre != "" && utf8.RuneCountInString(in.Nombre) <= 100 && emailValido(in.Email) && (in.Rol == "empleado" || in.Rol == "propietario") && (in.Estado == "activo" || in.Estado == "inactivo") && ((!nuevo && in.Password == "") || validarClave(in.Password))
}
func (a *app) listarUsuarios(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	rows, err := a.db.QueryContext(ctx, `SELECT id,nombre,email,rol,estado FROM usuarios ORDER BY nombre,id`)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	defer rows.Close()
	lista := []usuario{}
	for rows.Next() {
		var u usuario
		if err := rows.Scan(&u.ID, &u.Nombre, &u.Email, &u.Rol, &u.Estado); err != nil {
			errorInterno(w, r, err)
			return
		}
		lista = append(lista, u)
	}
	if err := rows.Err(); err != nil {
		errorInterno(w, r, err)
		return
	}
	escribirJSON(w, 200, lista)
}
func (a *app) crearUsuario(w http.ResponseWriter, r *http.Request) { a.guardarUsuario(w, r, true) }
func (a *app) actualizarUsuario(w http.ResponseWriter, r *http.Request) {
	a.guardarUsuario(w, r, false)
}
func (a *app) guardarUsuario(w http.ResponseWriter, r *http.Request, nuevo bool) {
	var in usuarioEntrada
	if !decodificarJSON(w, r, &in) {
		return
	}
	if !validarUsuario(&in, nuevo) {
		escribirError(w, 400, "nombre, correo, rol o estado inválidos; contraseña de 12 a 72 bytes")
		return
	}
	var id int64
	if !nuevo {
		var ok bool
		id, ok = idDeRuta(r)
		if !ok {
			escribirError(w, 400, "id inválido")
			return
		}
	}
	if !nuevo && id == usuarioActual(r).ID && (in.Estado != "activo" || in.Rol != "propietario") {
		escribirError(w, 409, "no puede desactivar su acceso ni quitarse el rol propietario")
		return
	}
	var hash []byte
	var err error
	if in.Password != "" {
		hash, err = bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
		if err != nil {
			errorInterno(w, r, err)
			return
		}
	}
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	if nuevo {
		res, err := a.db.ExecContext(ctx, `INSERT INTO usuarios(nombre,email,password_hash,rol,estado) VALUES(?,?,?,?,?)`, in.Nombre, in.Email, string(hash), in.Rol, in.Estado)
		if err != nil {
			errorSQLCRUD(w, r, err, "el correo ya está registrado", "")
			return
		}
		id, err = res.LastInsertId()
		if err != nil {
			errorInterno(w, r, err)
			return
		}
	} else {
		// La existencia no depende de RowsAffected: MySQL devuelve cero si no hay cambios.
		var existe int
		if err := a.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM usuarios WHERE id=?`, id).Scan(&existe); err != nil {
			errorInterno(w, r, err)
			return
		}
		if existe == 0 {
			escribirError(w, 404, "usuario no encontrado")
			return
		}
		_, err = a.db.ExecContext(ctx, `UPDATE usuarios SET nombre=?,email=?,rol=?,estado=?,password_hash=CASE WHEN ?='' THEN password_hash ELSE ? END WHERE id=?`, in.Nombre, in.Email, in.Rol, in.Estado, string(hash), string(hash), id)
		if err != nil {
			errorSQLCRUD(w, r, err, "el correo ya está registrado", "")
			return
		}
	}
	codigo := 200
	if nuevo {
		codigo = 201
	}
	escribirJSON(w, codigo, usuario{id, in.Nombre, in.Email, in.Rol, in.Estado})
}

// Herramientas de consola: usan la conexión existente, sin usuarios o claves predeterminados.
func configurarAcceso(db *sql.DB, crear, restablecer string) error {
	if (crear == "") == (restablecer == "") {
		return fmt.Errorf("use solo una opción de acceso")
	}
	email := strings.ToLower(strings.TrimSpace(crear))
	if restablecer != "" {
		email = strings.ToLower(strings.TrimSpace(restablecer))
	}
	if !emailValido(email) {
		return fmt.Errorf("correo inválido")
	}
	fmt.Fprintln(os.Stderr, "Contraseña por stdin (12 a 72 bytes):")
	clave, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && clave == "" {
		return err
	}
	clave = strings.TrimSuffix(strings.TrimSuffix(clave, "\n"), "\r")
	if !validarClave(clave) {
		return fmt.Errorf("la contraseña debe tener 12 a 72 bytes")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(clave), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if crear != "" {
		var n int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM usuarios WHERE rol='propietario' AND estado='activo' AND password_hash LIKE '$2%'`).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return fmt.Errorf("ya existe un propietario con acceso; use la administración de usuarios o -restablecer-clave")
		}
		_, err = db.ExecContext(ctx, `INSERT INTO usuarios(nombre,email,password_hash,rol,estado) VALUES('Propietario',?,?,'propietario','activo')`, email, string(hash))
	} else {
		var res sql.Result
		res, err = db.ExecContext(ctx, `UPDATE usuarios SET password_hash=? WHERE email=?`, string(hash), email)
		if err == nil {
			n, e := res.RowsAffected()
			if e != nil {
				return e
			}
			if n == 0 {
				return fmt.Errorf("correo no encontrado")
			}
		}
	}
	if err == nil {
		fmt.Fprintln(os.Stderr, "Acceso guardado en MySQL.")
	}
	return err
}
