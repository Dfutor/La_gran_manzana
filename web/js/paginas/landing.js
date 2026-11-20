// Landing pública: qué es La Gran Manzana y acceso al login.
import { h } from '../nucleo/dom.js';
import { icono } from '../nucleo/iconos.js';
import { url } from '../nucleo/router.js';
import { haySesion, usuarioActual } from '../nucleo/sesion.js';
import { marca } from '../nucleo/shell.js';

// Ilustración geométrica propia (SVG constante, sin datos de usuario).
const ILUSTRACION = `
<svg viewBox="0 0 480 420" xmlns="http://www.w3.org/2000/svg" role="img" aria-labelledby="ilus-t">
  <title id="ilus-t">Ilustración: manzana verde sobre cajas de inventario y una gráfica de ventas</title>
  <circle cx="250" cy="200" r="180" fill="#E3F1E6"/>
  <circle cx="390" cy="80" r="36" fill="#C8E6C9"/>
  <circle cx="80" cy="330" r="22" fill="#C8E6C9"/>
  <g transform="translate(60 250)">
    <rect width="150" height="110" rx="14" fill="#FFFFFF" stroke="#A9BBAE" stroke-width="2"/>
    <rect x="0" y="0" width="150" height="34" rx="14" fill="#2E7D32"/>
    <rect x="0" y="20" width="150" height="14" fill="#2E7D32"/>
    <rect x="18" y="52" width="70" height="10" rx="5" fill="#C8E6C9"/>
    <rect x="18" y="72" width="110" height="10" rx="5" fill="#E3F1E6"/>
    <rect x="18" y="90" width="90" height="10" rx="5" fill="#E3F1E6"/>
  </g>
  <g transform="translate(300 230)">
    <rect width="140" height="130" rx="14" fill="#FFFFFF" stroke="#A9BBAE" stroke-width="2"/>
    <rect x="20" y="78" width="18" height="34" rx="4" fill="#66BB6A"/>
    <rect x="48" y="58" width="18" height="54" rx="4" fill="#66BB6A"/>
    <rect x="76" y="40" width="18" height="72" rx="4" fill="#2E7D32"/>
    <rect x="104" y="22" width="18" height="90" rx="4" fill="#1B5E20"/>
    <path d="M20 40 L56 30 L86 18 L118 10" stroke="#F9A825" stroke-width="4" fill="none" stroke-linecap="round"/>
  </g>
  <g transform="translate(160 40)">
    <path d="M90 70 C60 40 0 44 0 120 C0 190 46 240 76 240 C88 240 94 232 110 232 C126 232 132 240 144 240 C174 240 220 190 220 120 C220 44 160 40 130 70 C120 80 100 80 90 70 Z" fill="#2E7D32"/>
    <path d="M60 110 C50 130 52 160 64 180" stroke="#66BB6A" stroke-width="12" fill="none" stroke-linecap="round" opacity=".8"/>
    <path d="M110 72 C108 40 120 18 140 6" stroke="#1B5E20" stroke-width="10" fill="none" stroke-linecap="round"/>
    <path d="M124 34 C150 6 196 14 198 28 C170 46 140 44 124 34 Z" fill="#66BB6A"/>
  </g>
  <g transform="translate(330 150)">
    <circle r="30" fill="#FFFFFF" stroke="#2E7D32" stroke-width="3"/>
    <path d="M-12 0 L-3 9 L13 -9" stroke="#2E7D32" stroke-width="5" fill="none" stroke-linecap="round" stroke-linejoin="round"/>
  </g>
</svg>`;

const QUE_ES = [
  { icono: 'boxes', titulo: 'Inventario por lotes', texto: 'Cada lote registra su factura y vencimiento. Al vender, el sistema descuenta primero lo que vence antes (FEFO) y nunca vende productos vencidos.' },
  { icono: 'heart-pulse', titulo: 'Atención orientada', texto: 'Cualquier persona del equipo puede buscar qué ofrecer según la necesidad del cliente, evitando contraindicaciones y con fichas claras.' },
  { icono: 'trending-up', titulo: 'Decisiones con datos', texto: 'Alertas de vencimiento y stock bajo, recomendaciones de qué hacer y reportes de ventas, pérdidas y rotación.' },
];

const MODULOS_LANDING = [
  { icono: 'shopping-cart', titulo: 'Ventas', texto: 'Efectivo, Nequi o Daviplata, con totales exactos y precios de oferta.' },
  { icono: 'package-plus', titulo: 'Ingreso de lotes', texto: 'Factura, vencimiento, unidades y costo de cada compra.' },
  { icono: 'bell', titulo: 'Alertas automáticas', texto: 'Qué liquidar, qué devolver y qué reponer, cada minuto.' },
  { icono: 'leaf', titulo: 'Consulta por necesidad', texto: 'Recomendaciones orientativas con aviso de complemento.' },
  { icono: 'chart-column', titulo: 'Reportes', texto: 'Ventas por día y producto, pérdidas y reposición sugerida.' },
  { icono: 'shield-check', titulo: 'Roles y permisos', texto: 'Cada persona ve solo lo que necesita para su trabajo.' },
];

export default function landing(raiz) {
  document.title = 'La Gran Manzana · Tienda naturista';
  const conSesion = haySesion();
  const destino = conSesion ? url('/app') : url('/login');
  const textoCta = conSesion ? 'Ir al panel' : 'Iniciar sesión';
  const cta = (clase = 'btn-primario btn-grande') => h('a', { class: 'btn ' + clase, href: destino, 'data-enlace': '' }, textoCta, icono('arrow-right', { tam: 18 }));

  const ilustracion = h('div', { class: 'ilustracion' });
  ilustracion.innerHTML = ILUSTRACION;

  raiz.append(
    h('header', { class: 'landing-barra' }, h('div', { class: 'envoltura' },
      marca(),
      h('nav', { 'aria-label': 'Acceso', class: 'fila' },
        h('a', { class: 'btn btn-fantasma', href: '#modulos', onclick: (e) => { e.preventDefault(); document.getElementById('modulos').scrollIntoView({ behavior: 'smooth' }); } }, 'Qué incluye'),
        cta('btn-primario')))),
    h('main', { id: 'contenido', tabindex: '-1' },
      h('section', { class: 'hero', 'aria-labelledby': 'hero-titulo' }, h('div', { class: 'envoltura' },
        h('div', {},
          h('span', { class: 'hero-eyebrow' }, icono('leaf', { tam: 16 }), 'Software interno · Tienda naturista'),
          h('h1', { id: 'hero-titulo' }, 'La Gran ', h('span', {}, 'Manzana')),
          h('p', { class: 'lema' }, 'El sistema del negocio para vender, controlar el inventario por lotes y atender a cada cliente con información confiable.'),
          h('div', { class: 'acciones' }, cta(), conSesion ? h('span', { class: 'texto-2 texto-peq', style: { alignSelf: 'center' } }, `Sesión activa: ${usuarioActual().nombre}`) : null)),
        ilustracion)),
      h('section', { class: 'seccion', 'aria-labelledby': 'que-es' }, h('div', { class: 'envoltura' },
        h('div', { class: 'seccion-titulo' }, h('h2', { id: 'que-es' }, 'Qué es'), h('p', {}, 'Una herramienta hecha a la medida de la tienda: rápida para el mostrador y clara para quien recién empieza.')),
        h('div', { class: 'bloques' }, QUE_ES.map((b) => h('article', { class: 'tarjeta bloque' },
          h('div', { class: 'bloque-icono' }, icono(b.icono, { tam: 24 })), h('h3', {}, b.titulo), h('p', {}, b.texto)))))),
      h('section', { class: 'seccion seccion-oscura', id: 'modulos', 'aria-labelledby': 'modulos-titulo' }, h('div', { class: 'envoltura' },
        h('div', { class: 'seccion-titulo' }, h('h2', { id: 'modulos-titulo' }, 'Lo que puede hacer'), h('p', {}, 'Los módulos se muestran según los permisos de cada persona.')),
        h('div', { class: 'modulos-landing' }, MODULOS_LANDING.map((m) => h('div', { class: 'modulo-landing' },
          h('div', { class: 'icono-caja' }, icono(m.icono, { tam: 20 })), h('div', {}, h('h3', { style: { fontSize: '16px' } }, m.titulo), h('p', {}, m.texto))))))),
      h('section', { class: 'seccion cta-final' }, h('div', { class: 'envoltura' }, h('div', { class: 'tarjeta' },
        h('h2', {}, conSesion ? 'Continúe donde lo dejó' : '¿Hace parte del equipo?'),
        h('p', { class: 'texto-2' }, conSesion ? 'Su panel tiene las recomendaciones del día.' : 'Ingrese con el correo y la contraseña que le asignó la administración.'),
        cta())))),
    h('footer', { class: 'pie' }, h('div', { class: 'envoltura' },
      h('div', { class: 'pila-1' }, h('strong', { style: { color: 'var(--color-texto)' } }, 'La Gran Manzana · Tienda naturista'), h('span', {}, 'Melgar, Tolima · Uso interno del negocio')),
      h('div', { class: 'fila' }, h('a', { href: url('/admin/login'), 'data-enlace': '' }, 'Acceso administrador'), h('span', { 'aria-hidden': 'true' }, '·'), h('span', {}, '© ' + new Date().getFullYear())))));
}
