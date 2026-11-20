package main

// Control de acceso basado en roles (RBAC) configurable.
//
//   Usuario → Roles → Permisos → Rutas del API
//
// Los permisos son acciones que el backend sabe proteger, por eso el catálogo
// vive en el código y se sincroniza con la tabla permisos al iniciar. Los roles
// y sus asignaciones son datos: el administrador los crea y modifica desde la
// interfaz sin desplegar. Un usuario recibe la unión de los permisos de sus
// roles activos; el middleware los recarga de MySQL en cada petición, así que
// los cambios aplican en la siguiente solicitud sin cerrar sesión.
//
// Reglas de administración de privilegios (quién puede otorgar qué):
//   - Nadie otorga, quita ni edita permisos que no tiene (sin escalada).
//   - Nadie modifica sus propios roles ni los roles que tiene asignados.
//   - Nadie modifica a un usuario con permisos que él no tiene.
//   - El rol Propietario es del sistema: tiene todos los permisos y no se edita.
//   - Siempre queda al menos un propietario activo.
//   - Cada cambio queda en auditoria_accesos dentro de la misma transacción.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"
)

type permisoDef struct {
	Nombre      string `json:"nombre"`
	Modulo      string `json:"modulo"`
	Descripcion string `json:"descripcion"`
	Critico     bool   `json:"critico"`
}

const (
	accesoPublico = "publico" // ruta sin sesión
	accesoSesion  = "sesion"  // cualquier usuario con sesión activa

	// Permisos de alcance: sin ellos, el usuario solo ve sus propios registros.
	permVentasVerTodas = "ventas.ver_todas"
	permBajasVerTodas  = "bajas.ver_todas"

	nombreRolPropietario = "Propietario"
	nombreRolEmpleado    = "Empleado"

	maxNombreRol      = 60
	maxDescripcionRol = 255
	maxPermisosRol    = 200
	maxRolesUsuario   = 50
)

var catalogoPermisos = []permisoDef{
	{"productos.ver", "Productos", "Consultar productos, fichas y plan FEFO", false},
	{"productos.crear", "Productos", "Registrar productos", false},
	{"productos.editar", "Productos", "Modificar productos y sus etiquetas", false},
	{"productos.eliminar", "Productos", "Desactivar productos", false},
	{"recomendaciones.consultar", "Recomendaciones", "Consultar productos según la necesidad del cliente", false},
	{"categorias.ver", "Categorías", "Consultar categorías", false},
	{"categorias.crear", "Categorías", "Registrar categorías", false},
	{"categorias.editar", "Categorías", "Modificar categorías", false},
	{"categorias.eliminar", "Categorías", "Eliminar categorías sin uso", false},
	{"proveedores.ver", "Proveedores", "Consultar proveedores", false},
	{"proveedores.crear", "Proveedores", "Registrar proveedores", false},
	{"proveedores.editar", "Proveedores", "Modificar proveedores", false},
	{"proveedores.eliminar", "Proveedores", "Eliminar proveedores sin uso", false},
	{"etiquetas.ver", "Etiquetas", "Consultar etiquetas y sus productos", false},
	{"etiquetas.crear", "Etiquetas", "Registrar etiquetas", false},
	{"etiquetas.editar", "Etiquetas", "Modificar etiquetas", false},
	{"etiquetas.eliminar", "Etiquetas", "Desactivar etiquetas", false},
	{"lotes.ver", "Lotes", "Consultar lotes", false},
	{"lotes.crear", "Lotes", "Registrar ingresos de lotes", false},
	{"lotes.editar", "Lotes", "Corregir código, factura o vencimiento de lotes", false},
	{"ventas.ver", "Ventas", "Consultar las ventas propias", false},
	{permVentasVerTodas, "Ventas", "Consultar las ventas de todos los usuarios", false},
	{"ventas.crear", "Ventas", "Registrar ventas", false},
	{"bajas.ver", "Bajas", "Consultar las bajas propias", false},
	{permBajasVerTodas, "Bajas", "Consultar las bajas de todos los usuarios", false},
	{"bajas.crear", "Bajas", "Registrar bajas de inventario", false},
	{"alertas.ver", "Alertas", "Consultar alertas y recomendaciones del panel", false},
	{"alertas.decidir", "Alertas", "Aceptar o descartar recomendaciones del panel", false},
	{"reportes.ventas", "Reportes", "Consultar el reporte de ventas", false},
	{"reportes.perdidas", "Reportes", "Consultar el reporte de pérdidas", false},
	{"reportes.rotacion", "Reportes", "Consultar el reporte de rotación y reposición", false},
	{"usuarios.ver", "Usuarios", "Consultar usuarios y sus permisos efectivos", false},
	{"usuarios.crear", "Usuarios", "Crear usuarios (sin roles)", true},
	{"usuarios.editar", "Usuarios", "Modificar datos, contraseña y estado de usuarios", true},
	{"roles.ver", "Roles y permisos", "Consultar roles y el catálogo de permisos", false},
	{"roles.gestionar", "Roles y permisos", "Crear, editar, desactivar y eliminar roles", true},
	{"roles.asignar", "Roles y permisos", "Asignar roles y permisos personalizados a usuarios", true},
	{"auditoria.ver", "Roles y permisos", "Consultar la auditoría de cambios de acceso", false},
}

// permisosEmpleado reproduce el acceso que tenía el rol fijo «empleado» antes de RBAC.
var permisosEmpleado = []string{
	"productos.ver", "recomendaciones.consultar", "categorias.ver", "proveedores.ver", "etiquetas.ver",
	"lotes.ver", "lotes.crear", "ventas.ver", permVentasVerTodas, "ventas.crear",
	"bajas.ver", permBajasVerTodas, "bajas.crear", "alertas.ver", "alertas.decidir",
}

var indicePermisos = func() map[string]permisoDef {
	m := make(map[string]permisoDef, len(catalogoPermisos))
	for _, p := range catalogoPermisos {
		m[p.Nombre] = p
	}
	return m
}()

// --- Tipos y utilidades ---

type rolResumen struct {
	ID     int64  `json:"id"`
	Nombre string `json:"nombre"`
	Activo bool   `json:"activo"`
}

type usuarioBreve struct {
	ID     int64  `json:"id"`
	Nombre string `json:"nombre"`
	Email  string `json:"email"`
	Estado string `json:"estado"`
}

type rolDetalle struct {
	ID               int64          `json:"id"`
	Nombre           string         `json:"nombre"`
	Descripcion      string         `json:"descripcion"`
	EsSistema        bool           `json:"es_sistema"`
	Activo           bool           `json:"activo"`
	Permisos         []string       `json:"permisos"`
	CantidadUsuarios int64          `json:"cantidad_usuarios"`
	Usuarios         []usuarioBreve `json:"usuarios,omitempty"`
}

// perfilUsuario es lo que ve el propio usuario: sus datos, roles y permisos efectivos.
type perfilUsuario struct {
	usuario
	Permisos []string `json:"permisos"`
}

// consultor lo cumplen *sql.DB y *sql.Tx.
type consultor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// errorHTTP permite abortar una transacción con una respuesta concreta.
type errorHTTP struct {
	codigo  int
	mensaje string
}

func (e errorHTTP) Error() string { return e.mensaje }
func fallo(codigo int, mensaje string) error {
	return errorHTTP{codigo, mensaje}
}

func responderFallo(w http.ResponseWriter, r *http.Request, err error, duplicado, fk string) {
	var e errorHTTP
	if errors.As(err, &e) {
		escribirError(w, e.codigo, e.mensaje)
		return
	}
	errorSQLCRUD(w, r, err, duplicado, fk)
}

func (a *app) transaccion(ctx context.Context, f func(tx *sql.Tx) error) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := f(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func conjunto(l []string) map[string]bool {
	m := make(map[string]bool, len(l))
	for _, s := range l {
		m[s] = true
	}
	return m
}

func ordenados(m map[string]bool) []string {
	l := make([]string, 0, len(m))
	for s, ok := range m {
		if ok {
			l = append(l, s)
		}
	}
	sort.Strings(l)
	return l
}

// faltantes devuelve, ordenados, los permisos requeridos que no están disponibles.
func faltantes(requeridos, disponibles map[string]bool) []string {
	var l []string
	for p, ok := range requeridos {
		if ok && !disponibles[p] {
			l = append(l, p)
		}
	}
	sort.Strings(l)
	return l
}

// diferencia devuelve lo que está en nuevo y no en antes, y viceversa.
func diferencia(antes, nuevo []string) (agregados, quitados []string) {
	a, n := conjunto(antes), conjunto(nuevo)
	return faltantes(n, a), faltantes(a, n)
}

func leerTextos(rows *sql.Rows, err error) ([]string, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	l := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		l = append(l, s)
	}
	return l, rows.Err()
}

func leerIDs(rows *sql.Rows, err error) ([]int64, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	l := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		l = append(l, id)
	}
	return l, rows.Err()
}

func comoArgs(ids []int64) []any {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return args
}

// --- Motor de autorización ---

// fuentesPermisos reúne, en una sola consulta, de dónde puede venir cada permiso
// de un usuario: sus roles activos y sus overrides.
type fuentesPermisos struct {
	deRoles     map[string]bool
	concedidos  map[string]bool
	denegados   map[string]bool
	propietario bool // tiene activo el rol del sistema
}

func leerFuentesPermisos(ctx context.Context, q consultor, usuarioID int64) (fuentesPermisos, error) {
	f := fuentesPermisos{map[string]bool{}, map[string]bool{}, map[string]bool{}, false}
	rows, err := q.QueryContext(ctx, `SELECT rp.permiso, IF(r.es_sistema=1,'sistema','rol') FROM usuario_roles ur JOIN roles r ON r.id=ur.rol_id AND r.activo=1 JOIN rol_permisos rp ON rp.rol_id=r.id WHERE ur.usuario_id=?
UNION ALL SELECT permiso, IF(concedido=1,'concedido','denegado') FROM usuario_permisos WHERE usuario_id=?`, usuarioID, usuarioID)
	if err != nil {
		return f, err
	}
	defer rows.Close()
	for rows.Next() {
		var p, fuente string
		if err := rows.Scan(&p, &fuente); err != nil {
			return f, err
		}
		switch fuente {
		case "sistema":
			f.propietario = true
			f.deRoles[p] = true
		case "rol":
			f.deRoles[p] = true
		case "concedido":
			f.concedidos[p] = true
		case "denegado":
			f.denegados[p] = true
		}
	}
	return f, rows.Err()
}

// efectivos aplica la prioridad: override del usuario > permiso del rol > denegado.
// El rol Propietario ignora las denegaciones: nunca se puede bloquear al administrador.
func (f fuentesPermisos) efectivos() map[string]bool {
	m := map[string]bool{}
	for p := range f.deRoles {
		m[p] = true
	}
	for p := range f.concedidos {
		m[p] = true
	}
	if !f.propietario {
		for p := range f.denegados {
			delete(m, p)
		}
	}
	return m
}

// permisosEfectivos: lo que el usuario puede hacer ahora mismo.
func permisosEfectivos(ctx context.Context, q consultor, usuarioID int64) (map[string]bool, error) {
	f, err := leerFuentesPermisos(ctx, q, usuarioID)
	return f.efectivos(), err
}

// permisosPotenciales: todo lo que el usuario podría tener (roles, incluso
// inactivos, más permisos concedidos). Sirve para impedir que alguien con
// menos acceso modifique a otro con más.
func permisosPotenciales(ctx context.Context, q consultor, usuarioID int64) (map[string]bool, []int64, error) {
	roles, err := rolesDeUsuario(ctx, q, usuarioID)
	if err != nil {
		return nil, nil, err
	}
	m, err := permisosDeRoles(ctx, q, roles)
	if err != nil {
		return nil, nil, err
	}
	concedidos, err := leerTextos(q.QueryContext(ctx, `SELECT permiso FROM usuario_permisos WHERE usuario_id=? AND concedido=1`, usuarioID))
	for _, p := range concedidos {
		m[p] = true
	}
	return m, roles, err
}

// permisosDeRoles incluye roles inactivos: sirve para decidir quién puede
// administrarlos, porque un rol inactivo puede reactivarse.
func permisosDeRoles(ctx context.Context, q consultor, ids []int64) (map[string]bool, error) {
	if len(ids) == 0 {
		return map[string]bool{}, nil
	}
	l, err := leerTextos(q.QueryContext(ctx, `SELECT DISTINCT permiso FROM rol_permisos WHERE rol_id IN (`+placeholders(len(ids))+`)`, comoArgs(ids)...))
	return conjunto(l), err
}

func rolesDeUsuario(ctx context.Context, q consultor, usuarioID int64) ([]int64, error) {
	return leerIDs(q.QueryContext(ctx, `SELECT rol_id FROM usuario_roles WHERE usuario_id=? ORDER BY rol_id`, usuarioID))
}

// rolesPorUsuario devuelve los roles de uno (id>0) o de todos los usuarios (id=0).
func rolesPorUsuario(ctx context.Context, q consultor, id int64) (map[int64][]rolResumen, error) {
	rows, err := q.QueryContext(ctx, `SELECT ur.usuario_id,r.id,r.nombre,r.activo FROM usuario_roles ur JOIN roles r ON r.id=ur.rol_id WHERE (?=0 OR ur.usuario_id=?) ORDER BY r.es_sistema DESC,r.nombre`, id, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[int64][]rolResumen{}
	for rows.Next() {
		var uid int64
		var rr rolResumen
		if err := rows.Scan(&uid, &rr.ID, &rr.Nombre, &rr.Activo); err != nil {
			return nil, err
		}
		m[uid] = append(m[uid], rr)
	}
	return m, rows.Err()
}

func (a *app) perfil(ctx context.Context, u usuario) (perfilUsuario, error) {
	roles, err := rolesPorUsuario(ctx, a.db, u.ID)
	if err != nil {
		return perfilUsuario{}, err
	}
	u.Roles = roles[u.ID]
	if u.Roles == nil {
		u.Roles = []rolResumen{}
	}
	return perfilUsuario{u, ordenados(u.permisos)}, nil
}

func registrarAuditoria(ctx context.Context, q consultor, actor usuario, accion, entidad string, entidadID int64, detalle any) error {
	b, err := json.Marshal(detalle)
	if err != nil {
		return err
	}
	var actorID, actorEmail, objetivo any
	if actor.ID > 0 {
		actorID, actorEmail = actor.ID, actor.Email
	}
	if entidadID > 0 {
		objetivo = entidadID
	}
	_, err = q.ExecContext(ctx, `INSERT INTO auditoria_accesos(actor_id,actor_email,accion,entidad,entidad_id,detalle) VALUES(?,?,?,?,?,?)`, actorID, actorEmail, accion, entidad, objetivo, string(b))
	return err
}

// rolPropietarioBloqueado bloquea la fila del rol del sistema. Toda operación que
// pueda dejar la tienda sin propietario pasa por aquí y queda serializada.
func rolPropietarioBloqueado(ctx context.Context, tx *sql.Tx) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM roles WHERE es_sistema=1 AND nombre=? FOR UPDATE`, nombreRolPropietario).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("no existe el rol %s; reinicie la aplicación para sincronizar el control de acceso", nombreRolPropietario)
	}
	return id, err
}

func otrosPropietariosActivos(ctx context.Context, tx *sql.Tx, rolID, excluir int64) (int, error) {
	var n int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM usuario_roles ur JOIN usuarios u ON u.id=ur.usuario_id WHERE ur.rol_id=? AND u.id<>? AND u.estado='activo' AND u.password_hash LIKE '$2%'`, rolID, excluir).Scan(&n)
	return n, err
}

// --- Sincronización al iniciar ---

var tablasRBAC = []string{"permisos", "roles", "rol_permisos", "usuario_roles", "auditoria_accesos", "usuario_permisos", "sugerencias_decisiones"}

// verificarEsquema solo lee: confirma que existan las tablas que crea
// docs/migracion_rbac.sql y que el usuario de la aplicación pueda verlas.
func verificarEsquema(ctx context.Context, db *sql.DB) error {
	presentes, err := leerTextos(db.QueryContext(ctx, `SELECT table_name FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN (`+placeholders(len(tablasRBAC))+`)`, comoArgsTexto(tablasRBAC)...))
	if err != nil {
		return err
	}
	hay := conjunto(presentes)
	var faltan []string
	for _, t := range tablasRBAC {
		if !hay[t] {
			faltan = append(faltan, t)
		}
	}
	if len(faltan) > 0 {
		return fmt.Errorf("faltan las tablas %s: ejecute docs/migracion_rbac.sql con un usuario administrador de MySQL", strings.Join(faltan, ", "))
	}
	return nil
}

// iniciarRBAC comprueba las tablas, sincroniza el catálogo de permisos y
// garantiza que el rol Propietario tenga todos. La primera vez (sin roles)
// crea Propietario y Empleado y asigna a cada usuario según su columna rol.
func iniciarRBAC(ctx context.Context, db *sql.DB) error {
	if err := verificarEsquema(ctx, db); err != nil {
		return err
	}
	a := &app{db: db}
	return a.transaccion(ctx, func(tx *sql.Tx) error {
		for _, p := range catalogoPermisos {
			if _, err := tx.ExecContext(ctx, `INSERT INTO permisos(nombre,modulo,descripcion) VALUES(?,?,?) ON DUPLICATE KEY UPDATE modulo=?,descripcion=?`, p.Nombre, p.Modulo, p.Descripcion, p.Modulo, p.Descripcion); err != nil {
				return err
			}
		}
		nombres := make([]string, len(catalogoPermisos))
		for i, p := range catalogoPermisos {
			nombres[i] = p.Nombre
		}
		res, err := tx.ExecContext(ctx, `DELETE FROM permisos WHERE nombre NOT IN (`+placeholders(len(nombres))+`)`, comoArgsTexto(nombres)...)
		if err != nil {
			return err
		}
		if borrados, _ := res.RowsAffected(); borrados > 0 {
			log.Printf("control de acceso: %d permisos retirados del catálogo", borrados)
		}
		var propietario int64
		err = tx.QueryRowContext(ctx, `SELECT id FROM roles WHERE es_sistema=1 AND nombre=? FOR UPDATE`, nombreRolPropietario).Scan(&propietario)
		if errors.Is(err, sql.ErrNoRows) {
			if propietario, err = inicializarRoles(ctx, tx); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT IGNORE INTO rol_permisos(rol_id,permiso) SELECT ?,nombre FROM permisos`, propietario)
		return err
	})
}

func comoArgsTexto(l []string) []any {
	args := make([]any, len(l))
	for i, s := range l {
		args[i] = s
	}
	return args
}

func inicializarRoles(ctx context.Context, tx *sql.Tx) (int64, error) {
	var existentes int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM roles`).Scan(&existentes); err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO roles(nombre,descripcion,es_sistema) VALUES(?,'Acceso total. Rol del sistema: no se puede editar ni eliminar.',1)`, nombreRolPropietario)
	if err != nil {
		return 0, fmt.Errorf("crear rol %s: %w", nombreRolPropietario, err)
	}
	propietario, err := res.LastInsertId()
	if err != nil || existentes > 0 {
		// Si ya había roles creados, solo se repone el rol del sistema.
		return propietario, err
	}
	res, err = tx.ExecContext(ctx, `INSERT INTO roles(nombre,descripcion) VALUES(?,'Operación diaria: catálogo, ventas, lotes, bajas y alertas.')`, nombreRolEmpleado)
	if err != nil {
		return 0, err
	}
	empleado, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err = insertarPermisosRol(ctx, tx, empleado, permisosEmpleado); err != nil {
		return 0, err
	}
	asignados, err := tx.ExecContext(ctx, `INSERT INTO usuario_roles(usuario_id,rol_id) SELECT id,IF(rol='propietario',?,?) FROM usuarios`, propietario, empleado)
	if err != nil {
		return 0, err
	}
	cantidad, _ := asignados.RowsAffected()
	log.Printf("control de acceso inicializado: roles %s y %s; %d usuarios asignados según su rol anterior", nombreRolPropietario, nombreRolEmpleado, cantidad)
	return propietario, registrarAuditoria(ctx, tx, usuario{}, "sistema.inicializar", "sistema", 0, map[string]any{"roles": []string{nombreRolPropietario, nombreRolEmpleado}, "usuarios_asignados": cantidad})
}

func insertarPermisosRol(ctx context.Context, q consultor, rolID int64, permisos []string) error {
	if len(permisos) == 0 {
		return nil
	}
	args := make([]any, 0, 2*len(permisos))
	for _, p := range permisos {
		args = append(args, rolID, p)
	}
	_, err := q.ExecContext(ctx, `INSERT INTO rol_permisos(rol_id,permiso) VALUES `+strings.TrimSuffix(strings.Repeat("(?,?),", len(permisos)), ","), args...)
	return err
}

// --- Validación ---

type rolEntrada struct {
	Nombre      *string  `json:"nombre"`
	Descripcion *string  `json:"descripcion"`
	Activo      *bool    `json:"activo"`
	Permisos    []string `json:"permisos"`
}

type rolValido struct {
	nombre, descripcion string
	activo              bool
	permisos            []string
}

func validarRol(in rolEntrada) (rolValido, string) {
	var v rolValido
	if in.Nombre == nil || strings.TrimSpace(*in.Nombre) == "" {
		return v, "el nombre del rol es obligatorio"
	}
	v.nombre = strings.TrimSpace(*in.Nombre)
	if utf8.RuneCountInString(v.nombre) > maxNombreRol {
		return v, fmt.Sprintf("el nombre admite máximo %d caracteres", maxNombreRol)
	}
	if in.Descripcion != nil {
		v.descripcion = strings.TrimSpace(*in.Descripcion)
	}
	if utf8.RuneCountInString(v.descripcion) > maxDescripcionRol {
		return v, fmt.Sprintf("la descripción admite máximo %d caracteres", maxDescripcionRol)
	}
	v.activo = in.Activo == nil || *in.Activo
	if in.Permisos == nil {
		return v, "permisos es obligatorio (lista de nombres; puede estar vacía)"
	}
	if len(in.Permisos) > maxPermisosRol {
		return v, fmt.Sprintf("máximo %d permisos", maxPermisosRol)
	}
	m := map[string]bool{}
	for _, p := range in.Permisos {
		if _, ok := indicePermisos[p]; !ok {
			return v, fmt.Sprintf("permiso desconocido: %q", p)
		}
		m[p] = true
	}
	v.permisos = ordenados(m)
	return v, ""
}

func validarIDsRoles(ids []int64) ([]int64, string) {
	if ids == nil {
		return nil, "roles es obligatorio (lista de IDs; puede estar vacía)"
	}
	if len(ids) > maxRolesUsuario {
		return nil, fmt.Sprintf("máximo %d roles", maxRolesUsuario)
	}
	vistos := map[int64]bool{}
	l := []int64{}
	for _, id := range ids {
		if id <= 0 {
			return nil, "los IDs de roles deben ser enteros positivos"
		}
		if !vistos[id] {
			vistos[id] = true
			l = append(l, id)
		}
	}
	slices.Sort(l)
	return l, ""
}

// --- Handlers: catálogo y roles ---

func (a *app) listarPermisos(w http.ResponseWriter, r *http.Request) {
	escribirJSON(w, 200, catalogoPermisos)
}

func (a *app) cargarRoles(ctx context.Context, q consultor, id int64) ([]rolDetalle, error) {
	rows, err := q.QueryContext(ctx, `SELECT r.id,r.nombre,r.descripcion,r.es_sistema,r.activo,(SELECT COUNT(*) FROM usuario_roles ur WHERE ur.rol_id=r.id) FROM roles r WHERE (?=0 OR r.id=?) ORDER BY r.es_sistema DESC,r.nombre`, id, id)
	if err != nil {
		return nil, err
	}
	lista := []rolDetalle{}
	indice := map[int64]int{}
	for rows.Next() {
		var d rolDetalle
		if err := rows.Scan(&d.ID, &d.Nombre, &d.Descripcion, &d.EsSistema, &d.Activo, &d.CantidadUsuarios); err != nil {
			rows.Close()
			return nil, err
		}
		d.Permisos = []string{}
		indice[d.ID] = len(lista)
		lista = append(lista, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows, err = q.QueryContext(ctx, `SELECT rol_id,permiso FROM rol_permisos WHERE (?=0 OR rol_id=?) ORDER BY rol_id,permiso`, id, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var rid int64
		var p string
		if err := rows.Scan(&rid, &p); err != nil {
			return nil, err
		}
		if i, ok := indice[rid]; ok {
			lista[i].Permisos = append(lista[i].Permisos, p)
		}
	}
	return lista, rows.Err()
}

func (a *app) listarRoles(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	lista, err := a.cargarRoles(ctx, a.db, 0)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	escribirJSON(w, 200, lista)
}

func (a *app) responderRol(w http.ResponseWriter, r *http.Request, id int64, codigo int) {
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	lista, err := a.cargarRoles(ctx, a.db, id)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	if len(lista) == 0 {
		escribirError(w, 404, "rol no encontrado")
		return
	}
	d := lista[0]
	rows, err := a.db.QueryContext(ctx, `SELECT u.id,u.nombre,u.email,u.estado FROM usuario_roles ur JOIN usuarios u ON u.id=ur.usuario_id WHERE ur.rol_id=? ORDER BY u.nombre,u.id`, id)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	defer rows.Close()
	d.Usuarios = []usuarioBreve{}
	for rows.Next() {
		var u usuarioBreve
		if err := rows.Scan(&u.ID, &u.Nombre, &u.Email, &u.Estado); err != nil {
			errorInterno(w, r, err)
			return
		}
		d.Usuarios = append(d.Usuarios, u)
	}
	if err := rows.Err(); err != nil {
		errorInterno(w, r, err)
		return
	}
	escribirJSON(w, codigo, d)
}

func (a *app) obtenerRol(w http.ResponseWriter, r *http.Request) {
	id, ok := idDeRuta(r)
	if !ok {
		escribirError(w, 400, "id inválido")
		return
	}
	a.responderRol(w, r, id, 200)
}

const msgRolDuplicado = "ya existe un rol con ese nombre"

func (a *app) crearRol(w http.ResponseWriter, r *http.Request) {
	var in rolEntrada
	if !decodificarJSON(w, r, &in) {
		return
	}
	v, problema := validarRol(in)
	if problema != "" {
		escribirError(w, 400, problema)
		return
	}
	actor := usuarioActual(r)
	if f := faltantes(conjunto(v.permisos), actor.permisos); len(f) > 0 {
		escribirError(w, 403, "no puede otorgar permisos que usted no tiene: "+strings.Join(f, ", "))
		return
	}
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	var id int64
	err := a.transaccion(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `INSERT INTO roles(nombre,descripcion,activo) VALUES(?,?,?)`, v.nombre, v.descripcion, v.activo)
		if err != nil {
			return err
		}
		if id, err = res.LastInsertId(); err != nil {
			return err
		}
		if err = insertarPermisosRol(ctx, tx, id, v.permisos); err != nil {
			return err
		}
		return registrarAuditoria(ctx, tx, actor, "rol.crear", "rol", id, map[string]any{"nombre": v.nombre, "descripcion": v.descripcion, "activo": v.activo, "permisos": v.permisos})
	})
	if err != nil {
		responderFallo(w, r, err, msgRolDuplicado, "")
		return
	}
	a.responderRol(w, r, id, 201)
}

// rolEditable bloquea el rol y aplica las reglas comunes de edición y eliminación.
func rolEditable(ctx context.Context, tx *sql.Tx, actor usuario, id int64) (rolDetalle, error) {
	var d rolDetalle
	err := tx.QueryRowContext(ctx, `SELECT id,nombre,descripcion,es_sistema,activo FROM roles WHERE id=? FOR UPDATE`, id).Scan(&d.ID, &d.Nombre, &d.Descripcion, &d.EsSistema, &d.Activo)
	if errors.Is(err, sql.ErrNoRows) {
		return d, fallo(404, "rol no encontrado")
	}
	if err != nil {
		return d, err
	}
	if d.EsSistema {
		return d, fallo(409, "el rol "+d.Nombre+" es del sistema: no se puede modificar ni eliminar")
	}
	var propio int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM usuario_roles WHERE usuario_id=? AND rol_id=?`, actor.ID, id).Scan(&propio); err != nil {
		return d, err
	}
	if propio > 0 {
		return d, fallo(409, "no puede modificar ni eliminar un rol que usted tiene asignado")
	}
	if d.Permisos, err = leerTextos(tx.QueryContext(ctx, `SELECT permiso FROM rol_permisos WHERE rol_id=? ORDER BY permiso`, id)); err != nil {
		return d, err
	}
	return d, tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM usuario_roles WHERE rol_id=?`, id).Scan(&d.CantidadUsuarios)
}

func (a *app) actualizarRol(w http.ResponseWriter, r *http.Request) {
	id, ok := idDeRuta(r)
	if !ok {
		escribirError(w, 400, "id inválido")
		return
	}
	var in rolEntrada
	if !decodificarJSON(w, r, &in) {
		return
	}
	v, problema := validarRol(in)
	if problema != "" {
		escribirError(w, 400, problema)
		return
	}
	actor := usuarioActual(r)
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	err := a.transaccion(ctx, func(tx *sql.Tx) error {
		antes, err := rolEditable(ctx, tx, actor, id)
		if err != nil {
			return err
		}
		// Solo se editan roles cuyos permisos (anteriores y nuevos) el actor también tiene.
		if f := faltantes(conjunto(append(slices.Clone(antes.Permisos), v.permisos...)), actor.permisos); len(f) > 0 {
			return fallo(403, "solo puede editar roles con permisos que usted tiene; le faltan: "+strings.Join(f, ", "))
		}
		if _, err = tx.ExecContext(ctx, `UPDATE roles SET nombre=?,descripcion=?,activo=? WHERE id=?`, v.nombre, v.descripcion, v.activo, id); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM rol_permisos WHERE rol_id=?`, id); err != nil {
			return err
		}
		if err = insertarPermisosRol(ctx, tx, id, v.permisos); err != nil {
			return err
		}
		cambios := map[string]any{}
		if antes.Nombre != v.nombre {
			cambios["nombre"] = []string{antes.Nombre, v.nombre}
		}
		if antes.Descripcion != v.descripcion {
			cambios["descripcion"] = []string{antes.Descripcion, v.descripcion}
		}
		if antes.Activo != v.activo {
			cambios["activo"] = []bool{antes.Activo, v.activo}
		}
		agregados, quitados := diferencia(antes.Permisos, v.permisos)
		if len(agregados) > 0 {
			cambios["permisos_agregados"] = agregados
		}
		if len(quitados) > 0 {
			cambios["permisos_quitados"] = quitados
		}
		if len(cambios) == 0 {
			return nil
		}
		cambios["rol"] = v.nombre
		cambios["usuarios_afectados"] = antes.CantidadUsuarios
		return registrarAuditoria(ctx, tx, actor, "rol.editar", "rol", id, cambios)
	})
	if err != nil {
		responderFallo(w, r, err, msgRolDuplicado, "")
		return
	}
	a.responderRol(w, r, id, 200)
}

func (a *app) eliminarRol(w http.ResponseWriter, r *http.Request) {
	id, ok := idDeRuta(r)
	if !ok {
		escribirError(w, 400, "id inválido")
		return
	}
	actor := usuarioActual(r)
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	err := a.transaccion(ctx, func(tx *sql.Tx) error {
		d, err := rolEditable(ctx, tx, actor, id)
		if err != nil {
			return err
		}
		if d.CantidadUsuarios > 0 {
			return fallo(409, fmt.Sprintf("el rol tiene %d usuarios asignados: quíteselo o desactívelo", d.CantidadUsuarios))
		}
		if f := faltantes(conjunto(d.Permisos), actor.permisos); len(f) > 0 {
			return fallo(403, "solo puede eliminar roles con permisos que usted tiene; le faltan: "+strings.Join(f, ", "))
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM roles WHERE id=?`, id); err != nil {
			return err
		}
		return registrarAuditoria(ctx, tx, actor, "rol.eliminar", "rol", id, map[string]any{"nombre": d.Nombre, "permisos": d.Permisos})
	})
	if err != nil {
		responderFallo(w, r, err, "", "el rol tiene usuarios asignados")
		return
	}
	w.WriteHeader(204)
}

// --- Handlers: asignación de roles y permisos efectivos ---

func (a *app) asignarRolesUsuario(w http.ResponseWriter, r *http.Request) {
	id, ok := idDeRuta(r)
	if !ok {
		escribirError(w, 400, "id inválido")
		return
	}
	var in struct {
		Roles []int64 `json:"roles"`
	}
	if !decodificarJSON(w, r, &in) {
		return
	}
	nuevos, problema := validarIDsRoles(in.Roles)
	if problema != "" {
		escribirError(w, 400, problema)
		return
	}
	actor := usuarioActual(r)
	if id == actor.ID {
		escribirError(w, 409, "no puede modificar sus propios roles; pídaselo a otro administrador")
		return
	}
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	err := a.transaccion(ctx, func(tx *sql.Tx) error {
		propietario, err := rolPropietarioBloqueado(ctx, tx)
		if err != nil {
			return err
		}
		var estado string
		err = tx.QueryRowContext(ctx, `SELECT estado FROM usuarios WHERE id=? FOR UPDATE`, id).Scan(&estado)
		if errors.Is(err, sql.ErrNoRows) {
			return fallo(404, "usuario no encontrado")
		}
		if err != nil {
			return err
		}
		anteriores, err := rolesDeUsuario(ctx, tx, id)
		if err != nil {
			return err
		}
		nombres := map[int64]string{}
		todos := slices.Compact(slices.Sorted(slices.Values(append(slices.Clone(anteriores), nuevos...))))
		if len(todos) > 0 {
			rows, err := tx.QueryContext(ctx, `SELECT id,nombre FROM roles WHERE id IN (`+placeholders(len(todos))+`)`, comoArgs(todos)...)
			if err != nil {
				return err
			}
			for rows.Next() {
				var rid int64
				var n string
				if err := rows.Scan(&rid, &n); err != nil {
					rows.Close()
					return err
				}
				nombres[rid] = n
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
		}
		for _, rid := range nuevos {
			if _, ok := nombres[rid]; !ok {
				return fallo(400, fmt.Sprintf("el rol %d no existe", rid))
			}
		}
		// Sin escalada: el actor debe tener todos los permisos de los roles que
		// toca y de los que el usuario ya tenía (no puede degradar a alguien con más acceso).
		permisos, err := permisosDeRoles(ctx, tx, todos)
		if err != nil {
			return err
		}
		concedidos, err := leerTextos(tx.QueryContext(ctx, `SELECT permiso FROM usuario_permisos WHERE usuario_id=? AND concedido=1`, id))
		if err != nil {
			return err
		}
		for _, p := range concedidos {
			permisos[p] = true
		}
		if f := faltantes(permisos, actor.permisos); len(f) > 0 {
			return fallo(403, "solo puede asignar o quitar roles a usuarios y con permisos que usted tiene; le faltan: "+strings.Join(f, ", "))
		}
		teniaPropietario, tendraPropietario := slices.Contains(anteriores, propietario), slices.Contains(nuevos, propietario)
		if teniaPropietario && !tendraPropietario && estado == "activo" {
			n, err := otrosPropietariosActivos(ctx, tx, propietario, id)
			if err != nil {
				return err
			}
			if n == 0 {
				return fallo(409, "debe quedar al menos un propietario activo")
			}
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM usuario_roles WHERE usuario_id=?`, id); err != nil {
			return err
		}
		if len(nuevos) > 0 {
			args := []any{}
			for _, rid := range nuevos {
				args = append(args, id, rid, actor.ID)
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO usuario_roles(usuario_id,rol_id,asignado_por) VALUES `+strings.TrimSuffix(strings.Repeat("(?,?,?),", len(nuevos)), ","), args...); err != nil {
				return err
			}
		}
		// La columna rol se mantiene para poder volver al binario anterior sin perder accesos.
		legado := "empleado"
		if tendraPropietario {
			legado = "propietario"
		}
		if _, err = tx.ExecContext(ctx, `UPDATE usuarios SET rol=? WHERE id=?`, legado, id); err != nil {
			return err
		}
		nombresDe := func(ids []int64) []string {
			l := []string{}
			for _, rid := range ids {
				l = append(l, nombres[rid])
			}
			sort.Strings(l)
			return l
		}
		agregados, quitados := []int64{}, []int64{}
		for _, rid := range nuevos {
			if !slices.Contains(anteriores, rid) {
				agregados = append(agregados, rid)
			}
		}
		for _, rid := range anteriores {
			if !slices.Contains(nuevos, rid) {
				quitados = append(quitados, rid)
			}
		}
		if len(agregados)+len(quitados) == 0 {
			return nil
		}
		return registrarAuditoria(ctx, tx, actor, "usuario.roles", "usuario", id, map[string]any{"roles_agregados": nombresDe(agregados), "roles_quitados": nombresDe(quitados)})
	})
	if err != nil {
		responderFallo(w, r, err, "", "")
		return
	}
	a.permisosUsuario(w, r)
}

type permisoEfectivo struct {
	permisoDef
	Roles []string `json:"roles"`
	// Origen: "rol" (heredado) o "personalizado" (override concedido).
	Origen string `json:"origen"`
}

// permisoDetalle describe cada permiso del catálogo para un usuario.
type permisoDetalle struct {
	permisoDef
	Roles    []string `json:"roles"`    // roles activos que lo otorgan
	Heredado bool     `json:"heredado"` // valor base según los roles
	Override string   `json:"override"` // "", "concedido" o "denegado"
	Efectivo bool     `json:"efectivo"`
}

// permisosUsuario muestra roles, overrides y permisos efectivos de un usuario,
// con el origen de cada permiso.
func (a *app) permisosUsuario(w http.ResponseWriter, r *http.Request) {
	id, ok := idDeRuta(r)
	if !ok {
		escribirError(w, 400, "id inválido")
		return
	}
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	var u usuarioBreve
	err := a.db.QueryRowContext(ctx, `SELECT id,nombre,email,estado FROM usuarios WHERE id=?`, id).Scan(&u.ID, &u.Nombre, &u.Email, &u.Estado)
	if errors.Is(err, sql.ErrNoRows) {
		escribirError(w, 404, "usuario no encontrado")
		return
	}
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	roles, err := rolesPorUsuario(ctx, a.db, id)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	rows, err := a.db.QueryContext(ctx, `SELECT rp.permiso,r.nombre FROM usuario_roles ur JOIN roles r ON r.id=ur.rol_id AND r.activo=1 JOIN rol_permisos rp ON rp.rol_id=r.id WHERE ur.usuario_id=? ORDER BY r.nombre`, id)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	defer rows.Close()
	origen := map[string][]string{}
	for rows.Next() {
		var p, rol string
		if err := rows.Scan(&p, &rol); err != nil {
			errorInterno(w, r, err)
			return
		}
		origen[p] = append(origen[p], rol)
	}
	if err := rows.Err(); err != nil {
		errorInterno(w, r, err)
		return
	}
	fuentes, err := leerFuentesPermisos(ctx, a.db, id)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	finales := fuentes.efectivos()
	efectivos := []permisoEfectivo{}
	detalle := []permisoDetalle{}
	overrides := []map[string]any{}
	for _, def := range catalogoPermisos {
		rs := origen[def.Nombre]
		if rs == nil {
			rs = []string{}
		}
		d := permisoDetalle{permisoDef: def, Roles: rs, Heredado: fuentes.deRoles[def.Nombre], Efectivo: finales[def.Nombre]}
		switch {
		case fuentes.concedidos[def.Nombre]:
			d.Override = "concedido"
		case fuentes.denegados[def.Nombre]:
			d.Override = "denegado"
		}
		if d.Override != "" {
			overrides = append(overrides, map[string]any{"permiso": def.Nombre, "concedido": d.Override == "concedido"})
		}
		detalle = append(detalle, d)
		if d.Efectivo {
			o := "rol"
			if !d.Heredado {
				o = "personalizado"
			}
			efectivos = append(efectivos, permisoEfectivo{def, rs, o})
		}
	}
	lista := roles[id]
	if lista == nil {
		lista = []rolResumen{}
	}
	escribirJSON(w, 200, map[string]any{
		"usuario": u, "activo": u.Estado == "activo", "es_propietario": fuentes.propietario,
		"roles": lista, "permisos": efectivos, "detalle": detalle, "overrides": overrides,
	})
}

type overrideEntrada struct {
	Permiso   string `json:"permiso"`
	Concedido *bool  `json:"concedido"`
}

func validarOverrides(l []overrideEntrada) (map[string]bool, string) {
	if l == nil {
		return nil, "overrides es obligatorio (lista; vacía restablece todo al rol)"
	}
	if len(l) > maxPermisosRol {
		return nil, fmt.Sprintf("máximo %d permisos personalizados", maxPermisosRol)
	}
	m := map[string]bool{}
	for _, o := range l {
		if _, ok := indicePermisos[o.Permiso]; !ok {
			return nil, fmt.Sprintf("permiso desconocido: %q", o.Permiso)
		}
		if o.Concedido == nil {
			return nil, "cada permiso personalizado necesita concedido: true o false"
		}
		if _, repetido := m[o.Permiso]; repetido {
			return nil, fmt.Sprintf("permiso repetido: %q", o.Permiso)
		}
		m[o.Permiso] = *o.Concedido
	}
	return m, ""
}

// PUT /api/usuarios/{id}/permisos {overrides:[{permiso,concedido}]} reemplaza los
// permisos personalizados del usuario. Lo que no aparece vuelve al valor del rol.
func (a *app) asignarPermisosUsuario(w http.ResponseWriter, r *http.Request) {
	id, ok := idDeRuta(r)
	if !ok {
		escribirError(w, 400, "id inválido")
		return
	}
	var in struct {
		Overrides []overrideEntrada `json:"overrides"`
	}
	if !decodificarJSON(w, r, &in) {
		return
	}
	nuevos, problema := validarOverrides(in.Overrides)
	if problema != "" {
		escribirError(w, 400, problema)
		return
	}
	actor := usuarioActual(r)
	if id == actor.ID {
		escribirError(w, 409, "no puede modificar sus propios permisos; pídaselo a otro administrador")
		return
	}
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	err := a.transaccion(ctx, func(tx *sql.Tx) error {
		var estado string
		err := tx.QueryRowContext(ctx, `SELECT estado FROM usuarios WHERE id=? FOR UPDATE`, id).Scan(&estado)
		if errors.Is(err, sql.ErrNoRows) {
			return fallo(404, "usuario no encontrado")
		}
		if err != nil {
			return err
		}
		potenciales, _, err := permisosPotenciales(ctx, tx, id)
		if err != nil {
			return err
		}
		if f := faltantes(potenciales, actor.permisos); len(f) > 0 {
			return fallo(403, "no puede modificar a un usuario con permisos que usted no tiene: "+strings.Join(f, ", "))
		}
		fuentes, err := leerFuentesPermisos(ctx, tx, id)
		if err != nil {
			return err
		}
		anteriores := map[string]bool{}
		for p := range fuentes.concedidos {
			anteriores[p] = true
		}
		for p := range fuentes.denegados {
			anteriores[p] = false
		}
		tocados := map[string]bool{}
		for p := range anteriores {
			tocados[p] = true
		}
		for p := range nuevos {
			tocados[p] = true
		}
		if f := faltantes(tocados, actor.permisos); len(f) > 0 {
			return fallo(403, "solo puede personalizar permisos que usted tiene; le faltan: "+strings.Join(f, ", "))
		}
		if fuentes.propietario {
			for p, concedido := range nuevos {
				if !concedido {
					return fallo(409, "el rol Propietario tiene acceso total: no admite permisos denegados ("+p+")")
				}
			}
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM usuario_permisos WHERE usuario_id=?`, id); err != nil {
			return err
		}
		if len(nuevos) > 0 {
			args := []any{}
			for _, p := range ordenados(tocados) {
				if concedido, ok := nuevos[p]; ok {
					args = append(args, id, p, concedido, actor.ID)
				}
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO usuario_permisos(usuario_id,permiso,concedido,asignado_por) VALUES `+strings.TrimSuffix(strings.Repeat("(?,?,?,?),", len(nuevos)), ","), args...); err != nil {
				return err
			}
		}
		cambios := map[string]any{}
		var concedidos, denegados, restablecidos []string
		for _, p := range ordenados(tocados) {
			antes, teniaAntes := anteriores[p]
			ahora, tieneAhora := nuevos[p]
			switch {
			case teniaAntes && !tieneAhora:
				restablecidos = append(restablecidos, p)
			case tieneAhora && (!teniaAntes || antes != ahora) && ahora:
				concedidos = append(concedidos, p)
			case tieneAhora && (!teniaAntes || antes != ahora):
				denegados = append(denegados, p)
			}
		}
		if len(concedidos) > 0 {
			cambios["permisos_concedidos"] = concedidos
		}
		if len(denegados) > 0 {
			cambios["permisos_denegados"] = denegados
		}
		if len(restablecidos) > 0 {
			cambios["restablecidos_al_rol"] = restablecidos
		}
		if len(cambios) == 0 {
			return nil
		}
		return registrarAuditoria(ctx, tx, actor, "usuario.permisos", "usuario", id, cambios)
	})
	if err != nil {
		responderFallo(w, r, err, "", "")
		return
	}
	a.permisosUsuario(w, r)
}

func (a *app) listarAuditoria(w http.ResponseWriter, r *http.Request) {
	limite, offset, err := paginacion(r)
	if err != nil {
		escribirError(w, 400, err.Error())
		return
	}
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	rows, err := a.db.QueryContext(ctx, `SELECT a.id,a.fecha,a.actor_id,COALESCE(u.nombre,a.actor_email,'sistema') AS actor,a.accion,a.entidad,a.entidad_id,a.detalle FROM auditoria_accesos a LEFT JOIN usuarios u ON u.id=a.actor_id ORDER BY a.id DESC LIMIT ? OFFSET ?`, limite, offset)
	responderFilas(w, r, rows, err)
}
