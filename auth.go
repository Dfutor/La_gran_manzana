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
	"log"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const nombreCookie = "granmanzana_sesion"
const duracionSesion = 8 * time.Hour

type usuario struct {
	ID     int64        `json:"id"`
	Nombre string       `json:"nombre"`
	Email  string       `json:"email"`
	Estado string       `json:"estado"`
	Roles  []rolResumen `json:"roles"`
	// permisos efectivos, cargados por el middleware en cada petición.
	permisos map[string]bool
}

func (u usuario) tiene(permiso string) bool { return u.permisos[permiso] }

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
	err = a.db.QueryRowContext(ctx, `SELECT id,nombre,email,estado,password_hash FROM usuarios WHERE email=?`, in.Email).Scan(&u.ID, &u.Nombre, &u.Email, &u.Estado, &hash)
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
	if u.permisos, err = permisosEfectivos(ctx, a.db, u.ID); err != nil {
		errorInterno(w, r, err)
		return
	}
	p, err := a.perfil(ctx, u)
	if err != nil {
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
	escribirJSON(w, 200, p)
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
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	p, err := a.perfil(ctx, usuarioActual(r))
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	escribirJSON(w, 200, p)
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

// protegerAPI autentica la sesión y autoriza cada ruta según el permiso que
// declara rutasAPI. Una ruta sin permiso declarado se niega (denegar por defecto).
func (a *app) protegerAPI(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			mux.ServeHTTP(w, r)
			return
		}
		_, patron := mux.Handler(r)
		permiso, declarada := permisoDeRuta[patron]
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
		if declarada && permiso == accesoPublico {
			mux.ServeHTTP(w, r)
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
		err = a.db.QueryRowContext(ctx, `SELECT id,nombre,email,estado,password_hash FROM usuarios WHERE id=?`, s.UsuarioID).Scan(&u.ID, &u.Nombre, &u.Email, &u.Estado, &hash)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			errorInterno(w, r, err)
			return
		}
		if err != nil || u.Estado != "activo" || hash != s.Hash {
			escribirError(w, 401, "sesión inválida; inicie sesión")
			return
		}
		// Sin caché: un cambio de roles o permisos aplica en la siguiente petición.
		if u.permisos, err = permisosEfectivos(ctx, a.db, u.ID); err != nil {
			errorInterno(w, r, err)
			return
		}
		if !declarada {
			log.Printf("[%s] ruta sin permiso declarado: %s %s", idPeticion(r.Context()), r.Method, r.URL.Path)
			escribirError(w, 403, "acceso denegado")
			return
		}
		if permiso != accesoSesion && !u.tiene(permiso) {
			escribirError(w, 403, "no tiene permiso para esta acción ("+permiso+")")
			return
		}
		mux.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), usuarioCtx{}, u)))
	})
}

type usuarioEntrada struct {
	Nombre   string `json:"nombre"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Estado   string `json:"estado"`
}

// Los roles no se reciben aquí: se asignan con PUT /api/usuarios/{id}/roles,
// que exige roles.asignar. Un usuario nuevo nace sin permisos (mínimo privilegio).
func validarUsuario(in *usuarioEntrada, nuevo bool) bool {
	in.Nombre = strings.TrimSpace(in.Nombre)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if in.Estado == "" {
		in.Estado = "activo"
	}
	return in.Nombre != "" && utf8.RuneCountInString(in.Nombre) <= 100 && emailValido(in.Email) && (in.Estado == "activo" || in.Estado == "inactivo") && ((!nuevo && in.Password == "") || validarClave(in.Password))
}
func (a *app) listarUsuarios(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	roles, err := rolesPorUsuario(ctx, a.db, 0)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	rows, err := a.db.QueryContext(ctx, `SELECT id,nombre,email,estado FROM usuarios ORDER BY nombre,id`)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	defer rows.Close()
	lista := []usuario{}
	for rows.Next() {
		var u usuario
		if err := rows.Scan(&u.ID, &u.Nombre, &u.Email, &u.Estado); err != nil {
			errorInterno(w, r, err)
			return
		}
		u.Roles = roles[u.ID]
		if u.Roles == nil {
			u.Roles = []rolResumen{}
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
	actor := usuarioActual(r)
	if !nuevo && id == actor.ID && in.Estado != "activo" {
		escribirError(w, 409, "no puede desactivar su propio acceso")
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
	err = a.transaccion(ctx, func(tx *sql.Tx) error {
		if nuevo {
			res, err := tx.ExecContext(ctx, `INSERT INTO usuarios(nombre,email,password_hash,rol,estado) VALUES(?,?,?,'empleado',?)`, in.Nombre, in.Email, string(hash), in.Estado)
			if err != nil {
				return err
			}
			if id, err = res.LastInsertId(); err != nil {
				return err
			}
			return registrarAuditoria(ctx, tx, actor, "usuario.crear", "usuario", id, map[string]any{"nombre": in.Nombre, "email": in.Email, "estado": in.Estado})
		}
		propietario, err := rolPropietarioBloqueado(ctx, tx)
		if err != nil {
			return err
		}
		var antes usuarioEntrada
		err = tx.QueryRowContext(ctx, `SELECT nombre,email,estado FROM usuarios WHERE id=? FOR UPDATE`, id).Scan(&antes.Nombre, &antes.Email, &antes.Estado)
		if errors.Is(err, sql.ErrNoRows) {
			return fallo(404, "usuario no encontrado")
		}
		if err != nil {
			return err
		}
		if id != actor.ID {
			// Cambiar correo, clave o estado de alguien con más permisos permitiría suplantarlo.
			permisos, roles, err := permisosPotenciales(ctx, tx, id)
			if err != nil {
				return err
			}
			if f := faltantes(permisos, actor.permisos); len(f) > 0 {
				return fallo(403, "no puede modificar a un usuario con permisos que usted no tiene: "+strings.Join(f, ", "))
			}
			if antes.Estado == "activo" && in.Estado != "activo" && slices.Contains(roles, propietario) {
				n, err := otrosPropietariosActivos(ctx, tx, propietario, id)
				if err != nil {
					return err
				}
				if n == 0 {
					return fallo(409, "debe quedar al menos un propietario activo")
				}
			}
		}
		if _, err = tx.ExecContext(ctx, `UPDATE usuarios SET nombre=?,email=?,estado=?,password_hash=CASE WHEN ?='' THEN password_hash ELSE ? END WHERE id=?`, in.Nombre, in.Email, in.Estado, string(hash), string(hash), id); err != nil {
			return err
		}
		cambios := map[string]any{}
		for campo, par := range map[string][2]string{"nombre": {antes.Nombre, in.Nombre}, "email": {antes.Email, in.Email}, "estado": {antes.Estado, in.Estado}} {
			if par[0] != par[1] {
				cambios[campo] = par
			}
		}
		if in.Password != "" {
			cambios["contraseña"] = "cambiada"
		}
		if len(cambios) == 0 {
			return nil
		}
		return registrarAuditoria(ctx, tx, actor, "usuario.editar", "usuario", id, cambios)
	})
	if err != nil {
		responderFallo(w, r, err, "el correo ya está registrado", "")
		return
	}
	roles, err := rolesPorUsuario(ctx, a.db, id)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	codigo := 200
	if nuevo {
		codigo = 201
	}
	u := usuario{ID: id, Nombre: in.Nombre, Email: in.Email, Estado: in.Estado, Roles: roles[id]}
	if u.Roles == nil {
		u.Roles = []rolResumen{}
	}
	escribirJSON(w, codigo, u)
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
		err = (&app{db: db}).transaccion(ctx, func(tx *sql.Tx) error {
			propietario, err := rolPropietarioBloqueado(ctx, tx)
			if err != nil {
				return err
			}
			n, err := otrosPropietariosActivos(ctx, tx, propietario, 0)
			if err != nil {
				return err
			}
			if n > 0 {
				return fmt.Errorf("ya existe un propietario con acceso; use la administración de usuarios o -restablecer-clave")
			}
			res, err := tx.ExecContext(ctx, `INSERT INTO usuarios(nombre,email,password_hash,rol,estado) VALUES('Propietario',?,?,'propietario','activo')`, email, string(hash))
			if err != nil {
				return err
			}
			id, err := res.LastInsertId()
			if err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO usuario_roles(usuario_id,rol_id) VALUES(?,?)`, id, propietario); err != nil {
				return err
			}
			return registrarAuditoria(ctx, tx, usuario{}, "usuario.crear", "usuario", id, map[string]any{"email": email, "origen": "consola", "roles_agregados": []string{nombreRolPropietario}})
		})
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
