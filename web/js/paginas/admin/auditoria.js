// Auditoría de cambios de acceso, del más reciente al más antiguo.
import { h, fechaHora } from '../../nucleo/dom.js';
import { get } from '../../nucleo/api.js';
import { tabla, insignia, vacio, esqueletoTabla, errorCarga, boton } from '../../nucleo/ui.js';

const ACCIONES = {
  'rol.crear': ['Creó un rol', 'exito'], 'rol.editar': ['Editó un rol', 'info'], 'rol.eliminar': ['Eliminó un rol', 'error'],
  'usuario.crear': ['Creó un usuario', 'exito'], 'usuario.editar': ['Editó un usuario', 'info'], 'usuario.roles': ['Cambió roles', 'advertencia'],
  'usuario.permisos': ['Personalizó permisos', 'advertencia'], 'sistema.inicializar': ['Inicializó roles', ''],
};
const CLAVES = {
  nombre: 'Nombre', descripcion: 'Descripción', activo: 'Activo', permisos: 'Permisos', permisos_agregados: 'Permisos agregados', permisos_quitados: 'Permisos quitados',
  roles_agregados: 'Roles agregados', roles_quitados: 'Roles quitados', usuarios_afectados: 'Usuarios afectados', rol: 'Rol', email: 'Correo', estado: 'Estado',
  'contraseña': 'Contraseña', permisos_concedidos: 'Permisos concedidos', permisos_denegados: 'Permisos denegados', restablecidos_al_rol: 'Restablecidos al rol',
  origen: 'Origen', roles: 'Roles', usuarios_asignados: 'Usuarios asignados',
};

function valorLegible(v) {
  if (Array.isArray(v)) return v.length === 2 && v.every((x) => typeof x !== 'object') && !v.every((x) => /\./.test(String(x))) ? `${v[0] === '' ? '(vacío)' : v[0]} → ${v[1]}` : v.join(', ');
  if (typeof v === 'boolean') return v ? 'sí' : 'no';
  return String(v);
}
function detalle(texto) {
  let datos;
  try { datos = typeof texto === 'string' ? JSON.parse(texto) : texto; } catch { return texto; }
  return h('ul', { class: 'lista-limpia texto-peq' }, Object.entries(datos || {}).map(([k, v]) => h('li', {}, h('span', { class: 'texto-2' }, (CLAVES[k] || k) + ': '), valorLegible(v))));
}

export default function auditoria(contenedor) {
  const LIMITE = 25;
  let offset = 0;
  const caja = h('div', {}, esqueletoTabla(8));
  const info = h('span', {});
  const ant = boton('Más recientes', { peq: true, onclick: () => { offset = Math.max(0, offset - LIMITE); cargar(); } });
  const sig = boton('Más antiguos', { peq: true, onclick: () => { offset += LIMITE; cargar(); } });

  async function cargar() {
    caja.replaceChildren(esqueletoTabla(8));
    const r = await get(`/api/auditoria?limite=${LIMITE}&offset=${offset}`);
    if (!r.ok) { caja.replaceChildren(errorCarga(r, cargar)); return; }
    ant.disabled = offset === 0;
    sig.disabled = r.data.length < LIMITE;
    info.textContent = r.data.length ? `Entradas ${offset + 1}–${offset + r.data.length}` : '';
    caja.replaceChildren(r.data.length ? tabla([
      { titulo: 'Fecha', valor: (e) => h('span', { class: 'num' }, fechaHora(e.fecha)) },
      { titulo: 'Quién', valor: (e) => h('span', { class: 'negrita' }, e.actor) },
      { titulo: 'Acción', valor: (e) => insignia(...(ACCIONES[e.accion] || [e.accion, ''])) },
      { titulo: 'Sobre', valor: (e) => `${e.entidad}${e.entidad_id ? ' #' + e.entidad_id : ''}` },
      { titulo: 'Detalle', valor: (e) => detalle(e.detalle) },
    ], r.data, { etiqueta: 'Auditoría' }) : vacio({ icono: 'scroll-text', titulo: 'Sin entradas' }),
    h('div', { class: 'paginacion' }, info, h('div', { class: 'fila' }, ant, sig)));
  }

  contenedor.append(
    h('div', { class: 'cabecera-pagina' }, h('div', {}, h('h2', {}, 'Auditoría'), h('p', {}, 'Registro inalterable de quién creó, editó o asignó roles y permisos, y de los cambios de usuarios.')),
      boton('Actualizar', { icono: 'refresh-cw', onclick: () => { offset = 0; cargar(); } })),
    h('div', { class: 'tarjeta tarjeta-sin-relleno' }, caja));
  cargar();
}
