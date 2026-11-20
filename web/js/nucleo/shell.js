// Layout de la aplicación autenticada: navegación lateral (cajón en móvil),
// encabezado con el título de la página y área de contenido.
import { h, iniciales, fechaLarga } from './dom.js';
import { icono } from './iconos.js';
import { MODULOS, MODULOS_ADMIN } from './modulos.js';
import { usuarioActual, puede, cerrarSesion } from './sesion.js';
import { url, navegar, rutaActual } from './router.js';
import { botonIcono } from './ui.js';

let shell = null;
let refs = {};

export const marca = (clase = '') => h('a', { class: 'marca ' + clase, href: url('/'), 'data-enlace': '' },
  h('span', { class: 'marca-logo' }, icono('apple', { tam: 20 })), 'La Gran Manzana');

export const modulosVisibles = (lista) => lista.filter((m) => !m.permisos.length || puede(...m.permisos));

function enlaceNav(m) {
  const a = h('a', { class: 'nav-enlace', href: url(m.ruta), 'data-enlace': '', 'data-modulo': m.id }, icono(m.icono, { tam: 20 }), h('span', {}, m.titulo));
  if (m.id === 'inicio') a.append(refs.contador = h('span', { class: 'contador', hidden: true, 'aria-label': 'recomendaciones pendientes' }));
  return a;
}

function construirNav() {
  const operacion = modulosVisibles(MODULOS);
  const admin = modulosVisibles(MODULOS_ADMIN);
  return h('nav', { 'aria-label': 'Principal' },
    h('div', { class: 'nav-grupo' }, h('p', { class: 'nav-grupo-titulo' }, 'Operación'), operacion.map(enlaceNav)),
    admin.length ? h('div', { class: 'nav-grupo' }, h('p', { class: 'nav-grupo-titulo' }, 'Administración'), admin.map(enlaceNav)) : null);
}

function abrirMenu(abrir) {
  shell.classList.toggle('menu-abierto', abrir);
  refs.botonMenu.setAttribute('aria-expanded', String(abrir));
  if (abrir) refs.lateral.querySelector('.nav-enlace')?.focus();
}

export function montarShell(raiz) {
  const u = usuarioActual();
  refs = {};
  refs.botonMenu = botonIcono('menu', 'Abrir menú', { class: 'boton-menu', 'aria-controls': 'lateral', 'aria-expanded': 'false', onclick: () => abrirMenu(!shell.classList.contains('menu-abierto')) });
  refs.titulo = h('h1', { class: 'titulo-pagina' });
  refs.contenido = h('main', { class: 'contenido', id: 'contenido', tabindex: '-1' });
  refs.nav = construirNav();
  const roles = u.roles.length ? u.roles.map((r) => r.nombre).join(', ') : 'Sin roles';
  refs.lateral = h('aside', { class: 'lateral', id: 'lateral' },
    marca(),
    refs.nav,
    h('div', { class: 'lateral-pie' },
      h('div', { class: 'usuario-tarjeta' }, h('span', { class: 'avatar', 'aria-hidden': 'true' }, iniciales(u.nombre)),
        h('div', { class: 'crece' }, h('div', { class: 'nombre' }, u.nombre), h('div', { class: 'roles' }, roles))),
      h('button', { type: 'button', class: 'btn btn-fantasma btn-salir', onclick: async () => { await cerrarSesion(); navegar('/login', { reemplazar: true }); } },
        icono('log-out', { tam: 18 }), 'Cerrar sesión')));
  shell = h('div', { class: 'shell' },
    refs.lateral,
    h('div', { class: 'velo', onclick: () => abrirMenu(false) }),
    h('div', { class: 'principal' },
      h('header', { class: 'encabezado' }, refs.botonMenu, refs.titulo, h('span', { class: 'fecha' }, fechaLarga())),
      refs.contenido));
  shell.addEventListener('keydown', (e) => { if (e.key === 'Escape' && shell.classList.contains('menu-abierto')) { abrirMenu(false); refs.botonMenu.focus(); } });
  raiz.replaceChildren(shell);
  return refs.contenido;
}

export const shellMontado = () => shell !== null && shell.isConnected;
export function desmontarShell() { shell = null; }

// Marca el enlace activo, actualiza el título y cierra el menú móvil.
export function prepararPagina(titulo, idModulo) {
  refs.titulo.textContent = titulo;
  document.title = `${titulo} · La Gran Manzana`;
  for (const a of refs.lateral.querySelectorAll('.nav-enlace')) {
    if (a.dataset.modulo === idModulo) a.setAttribute('aria-current', 'page');
    else a.removeAttribute('aria-current');
  }
  if (shell.classList.contains('menu-abierto')) abrirMenu(false);
  refs.contenido.replaceChildren();
  return refs.contenido;
}

export function actualizarContador(n) {
  if (!refs.contador) return;
  refs.contador.hidden = !n;
  refs.contador.textContent = n > 99 ? '99+' : String(n);
}

export const rutaEsAdmin = () => rutaActual().startsWith('/admin');
