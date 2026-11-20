// Sesión y permisos del usuario. La interfaz solo oculta lo que el usuario no
// puede usar; el servidor vuelve a validar cada permiso en el API.

import { api, eventosApi } from './api.js';

let usuario = null;
export const eventosSesion = new EventTarget();

export const usuarioActual = () => usuario;
export const haySesion = () => usuario !== null;

// puede('a', 'b'): true si tiene alguno de los permisos.
export function puede(...permisos) {
  const propios = usuario?.permisos || [];
  return permisos.flat().some((p) => propios.includes(p));
}

// Permisos que hacen a alguien «administrador» a efectos del panel de administración.
export const PERMISOS_ADMIN = ['usuarios.ver', 'usuarios.crear', 'usuarios.editar', 'roles.ver', 'roles.gestionar', 'roles.asignar', 'auditoria.ver'];
export const esAdmin = () => puede(...PERMISOS_ADMIN);

function fijar(u, detalle = {}) {
  const antes = JSON.stringify(usuario && { p: usuario.permisos, r: usuario.roles });
  usuario = u;
  const despues = JSON.stringify(usuario && { p: usuario.permisos, r: usuario.roles });
  if (antes !== despues) eventosSesion.dispatchEvent(new CustomEvent('cambio', { detail: detalle }));
}

export async function cargarSesion() {
  const r = await api('/api/sesion');
  fijar(r.ok ? r.data : null);
  return usuario;
}

export async function iniciarSesion(email, password) {
  const r = await api('/api/login', { metodo: 'POST', cuerpo: { email, password } });
  if (r.ok) fijar(r.data);
  return r;
}

export async function cerrarSesion() {
  await api('/api/logout', { metodo: 'POST' });
  fijar(null, { voluntario: true });
}

// Refresca roles y permisos sin cerrar la sesión (por ejemplo, tras un 403).
let refrescando = null;
export function refrescarSesion() {
  if (!usuario) return Promise.resolve(null);
  refrescando ??= cargarSesion().finally(() => { refrescando = null; });
  return refrescando;
}

eventosApi.addEventListener('sesion-vencida', () => { if (usuario) fijar(null); });
eventosApi.addEventListener('sin-permiso', () => refrescarSesion());
