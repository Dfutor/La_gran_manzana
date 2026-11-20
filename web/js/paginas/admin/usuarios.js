// Administración de usuarios: búsqueda, filtros, paginación, alta de cuentas y acciones por fila.
import { h, iniciales, normalizar } from '../../nucleo/dom.js';
import { icono } from '../../nucleo/iconos.js';
import { get, post, put } from '../../nucleo/api.js';
import { puede, usuarioActual } from '../../nucleo/sesion.js';
import { url } from '../../nucleo/router.js';
import { tabla, insignia, vacio, esqueletoTabla, errorCarga, boton, botonIcono, campo, notificar, modalFormulario, confirmar, selector, paginador, aviso } from '../../nucleo/ui.js';

// Contraseña inicial aleatoria y legible (sin caracteres ambiguos).
function generarClave() {
  const alfabeto = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789';
  const bytes = crypto.getRandomValues(new Uint8Array(14));
  return [...bytes].map((b) => alfabeto[b % alfabeto.length]).join('');
}

function campoClave(nombre, requerido) {
  const input = h('input', { name: nombre, type: 'text', minlength: '12', maxlength: '72', required: requerido, autocomplete: 'new-password', spellcheck: 'false' });
  const generar = boton('Generar', { peq: true, icono: 'key-round', onclick: () => { input.value = generarClave(); input.select(); } });
  return h('div', { class: 'pila-1' },
    campo(requerido ? 'Contraseña inicial' : 'Nueva contraseña (opcional)', input, { ayuda: '12 a 72 caracteres. Compártala con la persona por un canal seguro.' }),
    h('div', {}, generar));
}

export default async function usuarios(contenedor) {
  const yo = usuarioActual();
  const buscar = h('input', { type: 'search', placeholder: 'Nombre o correo', 'aria-label': 'Buscar usuarios' });
  const filtroRol = selector([{ valor: '', texto: 'Todos los roles' }], { 'aria-label': 'Filtrar por rol' });
  const filtroEstado = selector([{ valor: '', texto: 'Todos los estados' }, { valor: 'activo', texto: 'Activos' }, { valor: 'inactivo', texto: 'Inactivos' }], { 'aria-label': 'Filtrar por estado' });
  const caja = h('div', {}, esqueletoTabla());
  const pag = paginador(10, () => pintar());
  let lista = [];
  let roles = [];

  function pintar() {
    const q = normalizar(buscar.value);
    const filtrados = lista.filter((u) =>
      (!q || normalizar(u.nombre + ' ' + u.email).includes(q)) &&
      (!filtroRol.value || (filtroRol.value === 'ninguno' ? !u.roles.length : u.roles.some((r) => String(r.id) === filtroRol.value))) &&
      (!filtroEstado.value || u.estado === filtroEstado.value));
    if (!filtrados.length) { caja.replaceChildren(vacio({ icono: 'users', titulo: 'No hay usuarios con esos filtros' })); return; }
    caja.replaceChildren(tabla([
      { titulo: 'Usuario', valor: (u) => h('div', { class: 'fila', style: { flexWrap: 'nowrap' } }, h('span', { class: 'avatar', 'aria-hidden': 'true' }, iniciales(u.nombre)),
        h('span', {}, h('span', { class: 'negrita' }, u.nombre, u.id === yo.id ? ' (usted)' : ''), h('span', { class: 'sub' }, u.email))) },
      { titulo: 'Roles', valor: (u) => (u.roles.length ? h('div', { class: 'fila' }, u.roles.map((r) => insignia(r.nombre + (r.activo ? '' : ' (inactivo)'), r.activo ? 'exito' : ''))) : h('span', { class: 'texto-2 texto-peq' }, 'Sin roles')) },
      { titulo: 'Estado', valor: (u) => (u.estado === 'activo' ? insignia('Activo', 'exito', 'circle-check') : insignia('Inactivo', 'error', 'ban')) },
      { titulo: '', clase: 'derecha', valor: (u) => h('div', { class: 'acciones' },
        puede('usuarios.editar') ? botonIcono('pencil', `Editar ${u.nombre}`, { onclick: () => editar(u) }) : null,
        puede('usuarios.editar') && u.id !== yo.id ? botonIcono(u.estado === 'activo' ? 'user-x' : 'user-check', `${u.estado === 'activo' ? 'Desactivar' : 'Activar'} ${u.nombre}`, { onclick: () => cambiarEstado(u) }) : null,
        h('a', { class: 'btn btn-secundario btn-peq', href: url('/admin/usuarios/' + u.id), 'data-enlace': '' }, icono('shield-check', { tam: 16 }), 'Permisos')) },
    ], pag.pagina(filtrados), { etiqueta: 'Usuarios' }), pag.pie);
  }

  async function cargar() {
    caja.replaceChildren(esqueletoTabla());
    const [ru, rr] = await Promise.all([get('/api/usuarios'), puede('roles.ver') ? get('/api/roles') : Promise.resolve({ ok: true, data: [] })]);
    if (!ru.ok) { caja.replaceChildren(errorCarga(ru, cargar)); return; }
    lista = ru.data;
    roles = rr.ok ? rr.data : [];
    const actual = filtroRol.value;
    filtroRol.replaceChildren(h('option', { value: '' }, 'Todos los roles'), h('option', { value: 'ninguno' }, 'Sin roles'), ...roles.map((r) => h('option', { value: r.id }, r.nombre)));
    filtroRol.value = actual;
    pintar();
  }

  function crear() {
    const asigna = puede('roles.asignar') && roles.length;
    modalFormulario({
      titulo: 'Crear cuenta', descripcion: 'La persona ingresará con este correo y la contraseña inicial.',
      campos: h('div', { class: 'pila' },
        campo('Nombre completo', h('input', { name: 'nombre', required: true, maxlength: '100', autocomplete: 'off' })),
        campo('Correo', h('input', { name: 'email', type: 'email', required: true, maxlength: '150', autocomplete: 'off' })),
        campoClave('password', true),
        asigna
          ? campo('Rol inicial', selector([{ valor: '', texto: 'Sin rol (sin permisos)' }, ...roles.filter((r) => r.activo).map((r) => ({ valor: r.id, texto: `${r.nombre} · ${r.permisos.length} permisos` }))], { name: 'rol' }), { ayuda: 'Solo puede asignar roles con permisos que usted tiene.' })
          : aviso('La cuenta se creará sin roles. Asígnele permisos después desde «Permisos».', 'info')),
      textoEnviar: 'Crear cuenta',
      enviar: async (f) => {
        const e = f.elements;
        const r = await post('/api/usuarios', { nombre: e.nombre.value, email: e.email.value, password: e.password.value, estado: 'activo' });
        if (!r.ok) return r;
        if (e.rol?.value) {
          const ra = await put(`/api/usuarios/${r.data.id}/roles`, { roles: [Number(e.rol.value)] });
          if (!ra.ok) notificar(`Cuenta creada, pero no se asignó el rol: ${ra.error}`, 'error', 8000);
        }
        notificar(`Cuenta de ${r.data.nombre} creada.`, 'exito');
        cargar();
        return r;
      },
    });
  }

  function editar(u) {
    const propio = u.id === yo.id;
    modalFormulario({
      titulo: `Editar a ${u.nombre}`,
      campos: h('div', { class: 'pila' },
        campo('Nombre completo', h('input', { name: 'nombre', required: true, maxlength: '100', value: u.nombre })),
        campo('Correo', h('input', { name: 'email', type: 'email', required: true, maxlength: '150', value: u.email })),
        campo('Estado', selector([{ valor: 'activo', texto: 'Activo', seleccionado: u.estado === 'activo' }, { valor: 'inactivo', texto: 'Inactivo (no puede ingresar)', seleccionado: u.estado === 'inactivo' }], { name: 'estado', disabled: propio }),
          { ayuda: propio ? 'No puede desactivar su propio acceso.' : '' }),
        campoClave('password', false)),
      enviar: async (f) => {
        const e = f.elements;
        const r = await put('/api/usuarios/' + u.id, { nombre: e.nombre.value, email: e.email.value, estado: propio ? 'activo' : e.estado.value, password: e.password.value });
        if (r.ok) { notificar('Usuario actualizado.', 'exito'); cargar(); }
        return r;
      },
    });
  }

  async function cambiarEstado(u) {
    const desactivar = u.estado === 'activo';
    const ok = await confirmar({
      titulo: desactivar ? 'Desactivar usuario' : 'Activar usuario',
      mensaje: desactivar ? `${u.nombre} no podrá ingresar y sus sesiones abiertas se cerrarán en la siguiente acción. Sus roles se conservan.` : `${u.nombre} podrá volver a ingresar con sus roles actuales.`,
      confirmar: desactivar ? 'Desactivar' : 'Activar', peligro: desactivar,
    });
    if (!ok) return;
    const r = await put('/api/usuarios/' + u.id, { nombre: u.nombre, email: u.email, estado: desactivar ? 'inactivo' : 'activo', password: '' });
    notificar(r.ok ? `${u.nombre} ${desactivar ? 'desactivado' : 'activado'}.` : r.error, r.ok ? 'exito' : 'error');
    if (r.ok) cargar();
  }

  [buscar, filtroRol, filtroEstado].forEach((c) => c.addEventListener(c === buscar ? 'input' : 'change', () => { pag.reiniciar(); pintar(); }));
  contenedor.append(
    h('div', { class: 'cabecera-pagina' }, h('div', {}, h('h2', {}, 'Usuarios'), h('p', {}, 'Cree cuentas, active o desactive accesos y revise los permisos de cada persona.')),
      puede('usuarios.crear') ? boton('Crear cuenta', { tipo: 'primario', icono: 'user-plus', onclick: crear }) : null),
    h('div', { class: 'tarjeta tarjeta-sin-relleno' },
      h('div', { class: 'herramientas', style: { padding: '16px 16px 0' } },
        h('div', { class: 'campo busqueda' }, h('div', { class: 'entrada-icono' }, icono('search', { tam: 18 }), buscar)),
        h('div', { class: 'campo' }, filtroRol), h('div', { class: 'campo' }, filtroEstado)),
      caja));
  await cargar();
}
