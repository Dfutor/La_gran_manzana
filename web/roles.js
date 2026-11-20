'use strict';

// ---------- Roles y permisos (RBAC) ----------
// La interfaz deshabilita lo que el usuario no puede otorgar, pero quien decide
// es el servidor: toda regla se valida de nuevo en el API.

let catalogoPermisos = [];
let rolesCargados = [];

async function armarAccesos() {
  const necesarios = [];
  if (puede('roles.ver')) necesarios.push(api('/api/permisos'), api('/api/roles'));
  const [permisos, roles] = await Promise.all(necesarios);
  if (permisos && permisos.ok) catalogoPermisos = permisos.data;
  if (roles && roles.ok) rolesCargados = roles.data;
  if (puede('roles.ver')) armarRoles(permisos?.ok && roles?.ok ? null : mensajeError(permisos?.ok ? roles : permisos));
  if (puede('usuarios.ver')) armarAsignacion();
  if (puede('auditoria.ver')) armarAuditoria();
}

async function recargarRoles() {
  const r = await api('/api/roles');
  if (r.ok) rolesCargados = r.data;
  armarRoles(r.ok ? null : mensajeError(r));
}

// --- Lista de roles y editor ---

function armarRoles(error) {
  const caja = $('#accesos-roles');
  const salida = h('p', { role: 'status', class: error ? 'err' : null }, error || '');
  const gestiona = puede('roles.gestionar');
  const tabla = h('table', {},
    h('thead', {}, h('tr', {}, ['Rol', 'Descripción', 'Estado', 'Permisos', 'Usuarios', ''].map((t) => h('th', {}, t)))),
    h('tbody', {}, rolesCargados.map((r) => h('tr', { class: r.activo ? null : 'inactivo' },
      h('td', {}, r.nombre, r.es_sistema ? h('small', {}, ' (sistema)') : null),
      h('td', {}, texto(r.descripcion)),
      h('td', {}, r.activo ? 'activo' : 'inactivo'),
      h('td', {}, `${r.permisos.length} de ${catalogoPermisos.length}`),
      h('td', {}, r.cantidad_usuarios),
      h('td', {},
        h('button', { type: 'button', onclick: () => editarRol(r, false) }, r.es_sistema || !gestiona ? 'Ver' : 'Editar'), ' ',
        gestiona ? h('button', { type: 'button', onclick: () => editarRol(r, true) }, 'Duplicar') : null, ' ',
        gestiona && !r.es_sistema ? h('button', { type: 'button', onclick: () => eliminarRol(r, salida) }, 'Eliminar') : null),
    ))));
  caja.replaceChildren(h('div', {},
    h('h3', {}, 'Roles'),
    gestiona ? h('button', { type: 'button', onclick: () => editarRol(null, false) }, 'Nuevo rol') : null,
    salida,
    h('div', { class: 'scroll' }, tabla),
    h('div', { id: 'editor-rol' }),
  ));
}

async function eliminarRol(r, salida) {
  if (!window.confirm(`¿Eliminar el rol «${r.nombre}»? Esta acción no se puede deshacer.`)) return;
  const res = await api('/api/roles/' + r.id, { method: 'DELETE' });
  await recargarRoles();
  msg($('#accesos-roles p[role="status"]') || salida, res.ok ? 'ok' : 'err', res.ok ? `Rol «${r.nombre}» eliminado.` : mensajeError(res));
}

// editarRol abre el formulario: rol null = nuevo; duplicar = nuevo con los permisos de r.
function editarRol(r, duplicar) {
  const nuevo = !r || duplicar;
  const soloLectura = !puede('roles.gestionar') || (!nuevo && r.es_sistema);
  const propios = new Set(usuarioSesion.permisos);
  const sinPermiso = r ? r.permisos.filter((p) => !propios.has(p)) : [];
  // Al duplicar se omiten los permisos que el usuario no puede otorgar.
  const marcados = new Set(r ? r.permisos.filter((p) => !duplicar || propios.has(p)) : []);
  const titulo = nuevo ? (duplicar ? `Nuevo rol (copia de «${r.nombre}»)` : 'Nuevo rol') : (soloLectura ? `Rol «${r.nombre}»` : `Editar rol «${r.nombre}»`);

  const nombre = h('input', { name: 'nombre', required: true, maxlength: 60, disabled: soloLectura });
  nombre.value = r ? (duplicar ? `${r.nombre} (copia)`.slice(0, 60) : r.nombre) : '';
  const descripcion = h('input', { name: 'descripcion', maxlength: 255, size: 50, disabled: soloLectura });
  descripcion.value = r ? r.descripcion : '';
  const activo = h('input', { type: 'checkbox', name: 'activo', disabled: soloLectura });
  activo.checked = r ? r.activo : true;

  const modulos = [];
  for (const p of catalogoPermisos) {
    let m = modulos.find((x) => x.nombre === p.modulo);
    if (!m) modulos.push(m = { nombre: p.modulo, permisos: [] });
    m.permisos.push(p);
  }
  const casillas = [];
  const matriz = h('div', { class: 'matriz-permisos' }, modulos.map((m) => {
    const propiasModulo = [];
    const fs = h('fieldset', {}, h('legend', {}, m.nombre),
      m.permisos.map((p) => {
        // No se puede otorgar ni quitar un permiso que uno no tiene.
        const c = h('input', { type: 'checkbox', value: p.nombre, disabled: soloLectura || !propios.has(p.nombre) });
        c.checked = marcados.has(p.nombre);
        casillas.push(c);
        propiasModulo.push(c);
        return h('label', { class: 'permiso', title: p.nombre }, c, ' ', p.descripcion,
          p.critico ? h('b', { class: 'critico' }, ' crítico') : null,
          h('br'), h('code', {}, p.nombre));
      }));
    if (!soloLectura) {
      fs.querySelector('legend').append(' ',
        h('button', { type: 'button', onclick: () => propiasModulo.forEach((c) => { if (!c.disabled) c.checked = true; }) }, 'todos'), ' ',
        h('button', { type: 'button', onclick: () => propiasModulo.forEach((c) => { if (!c.disabled) c.checked = false; }) }, 'ninguno'));
    }
    return fs;
  }));

  const salida = h('p', { role: 'status' });
  const guardar = h('button', { type: 'submit', hidden: soloLectura }, nuevo ? 'Crear rol' : 'Guardar cambios');
  const form = h('form', {},
    h('h3', {}, titulo),
    !nuevo && r.es_sistema ? h('p', { class: 'aviso' }, 'Rol del sistema: tiene todos los permisos y no se puede modificar ni eliminar.') : null,
    !soloLectura && sinPermiso.length && !duplicar ? h('p', { class: 'aviso' }, `Este rol incluye permisos que usted no tiene (${sinPermiso.join(', ')}); el servidor rechazará los cambios.`) : null,
    duplicar && sinPermiso.length ? h('p', { class: 'aviso' }, `Se omitieron permisos que usted no puede otorgar: ${sinPermiso.join(', ')}.`) : null,
    !nuevo && !soloLectura && r.cantidad_usuarios ? h('p', {}, h('small', {}, `Los cambios afectan de inmediato a ${r.cantidad_usuarios} usuario(s) con este rol.`)) : null,
    h('label', {}, 'Nombre ', nombre),
    h('label', {}, 'Descripción ', descripcion),
    h('label', {}, activo, ' Activo (un rol inactivo no otorga permisos)'),
    matriz,
    guardar, ' ',
    h('button', { type: 'button', onclick: () => $('#editor-rol').replaceChildren() }, 'Cerrar'),
    salida);

  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    const datos = {
      nombre: nombre.value, descripcion: descripcion.value, activo: activo.checked,
      permisos: casillas.filter((c) => c.checked).map((c) => c.value),
    };
    guardar.disabled = true;
    const res = await api('/api/roles' + (nuevo ? '' : '/' + r.id), { method: nuevo ? 'POST' : 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(datos) });
    guardar.disabled = false;
    if (!res.ok) { msg(salida, 'err', mensajeError(res)); return; }
    await recargarRoles();
    if (puede('usuarios.ver')) armarAsignacion();
    editarRol(res.data, false);
    msg($('#editor-rol p[role="status"]'), 'ok', `Rol «${res.data.nombre}» guardado con ${res.data.permisos.length} permisos.`);
  });

  $('#editor-rol').replaceChildren(form);
  form.scrollIntoView({ behavior: 'smooth' });
}

// --- Asignación de roles a usuarios ---

async function armarAsignacion() {
  const caja = $('#accesos-usuarios');
  const r = await api('/api/usuarios');
  if (!r.ok) { caja.replaceChildren(h('h3', {}, 'Usuarios'), h('p', { class: 'err' }, mensajeError(r))); return; }
  const selector = h('select', { 'aria-label': 'Usuario' },
    h('option', { value: '' }, 'Seleccione un usuario…'),
    r.data.map((u) => h('option', { value: u.id }, `${u.nombre} <${u.email}> — ${u.roles.length ? u.roles.map((x) => x.nombre).join(', ') : 'sin roles'}${u.estado === 'activo' ? '' : ' [inactivo]'}`)));
  const detalle = h('div', {});
  // Tras guardar, se actualiza el texto de la opción con los roles nuevos.
  detalle.addEventListener('roles-actualizados', (e) => {
    const u = e.detail;
    selector.selectedOptions[0].textContent = `${u.nombre} <${u.email}> — ${u.roles.length ? u.roles.map((x) => x.nombre).join(', ') : 'sin roles'}${u.estado === 'activo' ? '' : ' [inactivo]'}`;
  });
  selector.addEventListener('change', () => mostrarUsuario(Number(selector.value), detalle));
  caja.replaceChildren(h('h3', {}, 'Roles de cada usuario y permisos efectivos'), h('label', {}, 'Usuario ', selector), detalle);
}

async function mostrarUsuario(id, caja) {
  if (!id) { caja.replaceChildren(); return; }
  caja.replaceChildren(h('p', {}, 'Consultando…'));
  const r = await api(`/api/usuarios/${id}/permisos`);
  if (!r.ok) { caja.replaceChildren(h('p', { class: 'err' }, mensajeError(r))); return; }
  const { usuario: u, roles, permisos } = r.data;
  const propio = u.id === usuarioSesion.id;
  const asigna = puede('roles.asignar') && puede('roles.ver') && !propio;
  const actuales = new Set(roles.map((x) => x.id));
  const salida = h('p', { role: 'status' });

  let edicion = null;
  if (puede('roles.ver')) {
    const casillas = rolesCargados.map((rol) => {
      const c = h('input', { type: 'checkbox', value: rol.id, disabled: !asigna });
      c.checked = actuales.has(rol.id);
      return h('label', { class: 'permiso' }, c, ' ', rol.nombre, rol.activo ? '' : ' [inactivo]', h('small', {}, ` (${rol.permisos.length} permisos)`));
    });
    const guardar = h('button', { type: 'submit', hidden: !asigna }, 'Guardar roles');
    edicion = h('form', {},
      h('h4', {}, 'Roles asignados'),
      propio ? h('p', {}, h('small', {}, 'No puede modificar sus propios roles: otro administrador debe hacerlo.')) : null,
      !puede('roles.asignar') ? h('p', {}, h('small', {}, 'Necesita el permiso roles.asignar para cambiar roles.')) : null,
      h('div', { class: 'matriz-permisos' }, casillas), guardar, salida);
    edicion.addEventListener('submit', async (e) => {
      e.preventDefault();
      const ids = [...edicion.querySelectorAll('input:checked')].map((c) => Number(c.value));
      guardar.disabled = true;
      const res = await api(`/api/usuarios/${id}/roles`, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ roles: ids }) });
      guardar.disabled = false;
      if (!res.ok) { msg(salida, 'err', mensajeError(res)); return; }
      caja.dispatchEvent(new CustomEvent('roles-actualizados', { detail: { ...res.data.usuario, roles: res.data.roles } }));
      await mostrarUsuario(id, caja);
      msg(caja.querySelector('p[role="status"]'), 'ok', 'Roles actualizados. Aplican en la siguiente acción del usuario.');
      recargarRoles();
    });
  } else {
    edicion = h('p', {}, 'Roles: ', roles.length ? roles.map((x) => x.nombre).join(', ') : 'ninguno');
  }

  caja.replaceChildren(h('div', {},
    h('p', {}, h('b', {}, u.nombre), ` <${u.email}> — ${u.estado}`),
    edicion,
    h('h4', {}, `Permisos efectivos (${permisos.length})`),
    u.estado !== 'activo' ? h('p', { class: 'err' }, 'Usuario inactivo: no puede ingresar aunque tenga permisos.') : null,
    permisos.length
      ? h('div', { class: 'scroll' }, h('table', {},
        h('thead', {}, h('tr', {}, h('th', {}, 'Módulo'), h('th', {}, 'Permiso'), h('th', {}, 'Otorgado por'))),
        h('tbody', {}, permisos.map((p) => h('tr', {},
          h('td', {}, p.modulo),
          h('td', {}, p.descripcion, p.critico ? h('b', { class: 'critico' }, ' crítico') : null, h('br'), h('code', {}, p.nombre)),
          h('td', {}, p.roles.join(', ')))))))
      : h('p', {}, 'Sin permisos: no puede usar ninguna función.')));
}

// --- Auditoría ---

function armarAuditoria() {
  const lista = h('div', { class: 'scroll' });
  $('#accesos-auditoria').replaceChildren(
    h('h3', {}, 'Auditoría de cambios de acceso'),
    h('p', {}, h('small', {}, 'Creación, edición y eliminación de roles; asignación de roles; alta y cambios de usuarios. Las 100 entradas más recientes.')),
    h('button', { type: 'button', onclick: () => cargarTabla('/api/auditoria', lista) }, 'Consultar auditoría'),
    lista);
}
