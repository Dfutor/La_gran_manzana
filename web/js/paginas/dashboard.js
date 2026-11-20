// Panel principal: saludo, recomendaciones (elemento principal), métricas y
// accesos rápidos a los módulos habilitados.
import { h, pesos, numero, saludo, fechaLarga, hoyISO } from '../nucleo/dom.js';
import { icono } from '../nucleo/iconos.js';
import { get, post } from '../nucleo/api.js';
import { url, navegar } from '../nucleo/router.js';
import { usuarioActual, puede } from '../nucleo/sesion.js';
import { MODULOS } from '../nucleo/modulos.js';
import { modulosVisibles, actualizarContador } from '../nucleo/shell.js';
import { boton, insignia, vacio, esqueleto, errorCarga, notificar, cargando } from '../nucleo/ui.js';

const ICONO_TIPO = { vencido: 'ban', liquidar: 'clock', devolver: 'truck', destacar: 'sparkles', reponer: 'package-plus' };
const PRIORIDAD = { 1: ['Urgente', 'error'], 2: ['Pronto', 'advertencia'], 3: ['Planear', 'info'] };
const VISIBLES = 5;

// A dónde lleva «Aceptar» según la recomendación.
function rutaDestino(s) {
  if (s.destino === 'baja') return `/app/inventario?baja=${s.lote_id}&motivo=${s.tipo === 'vencido' ? 'vencimiento' : 'devolucion'}`;
  if (s.destino === 'lote') return `/app/inventario?ingreso=${s.producto_id}`;
  return `/app/productos?id=${s.producto_id}`;
}
function puedeIrA(s) {
  if (s.destino === 'baja') return puede('bajas.crear');
  if (s.destino === 'lote') return puede('lotes.crear');
  return puede('productos.ver');
}

function tarjetaRecomendaciones() {
  const lista = h('div', { class: 'lista-recos' });
  const contador = h('span', { class: 'insignia insignia-exito', hidden: true });
  const cuerpo = h('div', { 'aria-live': 'polite' }, cargando('Cargando recomendaciones…'), esqueleto({ lineas: 3, alto: 72 }));
  let todas = [];
  let mostrarTodas = false;

  function pintar() {
    actualizarContador(todas.length);
    contador.hidden = !todas.length;
    contador.textContent = `${todas.length} pendiente${todas.length === 1 ? '' : 's'}`;
    if (!todas.length) {
      cuerpo.replaceChildren(vacio({ icono: 'circle-check', titulo: 'Todo en orden', texto: 'No hay recomendaciones pendientes. Aparecerán aquí cuando un lote se acerque al vencimiento o un producto baje del mínimo.' }));
      return;
    }
    const visibles = mostrarTodas ? todas : todas.slice(0, VISIBLES);
    lista.replaceChildren(...visibles.map(item));
    const mas = todas.length > VISIBLES
      ? boton(mostrarTodas ? 'Mostrar menos' : `Mostrar ${todas.length - VISIBLES} más`, { peq: true, tipo: 'fantasma', icono: 'chevron-down', onclick: () => { mostrarTodas = !mostrarTodas; pintar(); } })
      : null;
    cuerpo.replaceChildren(lista, mas ? h('div', { style: { marginTop: '12px', textAlign: 'center' } }, mas) : '');
  }

  function quitar(s, el) {
    el.classList.add('saliendo');
    setTimeout(() => { todas = todas.filter((x) => x.clave !== s.clave); pintar(); }, 200);
  }

  async function decidir(s, decision, el, btn) {
    btn.disabled = true;
    const r = await post('/api/sugerencias/decision', { clave: s.clave, decision });
    if (!r.ok) {
      btn.disabled = false;
      notificar(r.error, 'error');
      if (r.status === 404 || r.status === 409) quitar(s, el);
      return;
    }
    quitar(s, el);
    if (decision === 'aceptada') {
      notificar('Recomendación aceptada. Complete la acción en la pantalla que se abrió.', 'exito');
      if (puedeIrA(s)) navegar(rutaDestino(s));
    } else {
      notificar('Recomendación descartada para todo el equipo.', 'info');
    }
  }

  function item(s) {
    const [textoPrioridad, tipoPrioridad] = PRIORIDAD[s.prioridad] || PRIORIDAD[3];
    const el = h('article', { class: `reco reco-p${s.prioridad}`, 'aria-labelledby': 'reco-' + s.clave.replace(/\W/g, '-') });
    const botones = h('div', { class: 'botones' });
    if (puede('alertas.decidir')) {
      const aceptar = boton('Aceptar', { tipo: 'primario', peq: true, icono: 'check', 'aria-label': `Aceptar: ${s.titulo}` });
      const descartar = boton('Descartar', { peq: true, 'aria-label': `Descartar: ${s.titulo}` });
      aceptar.addEventListener('click', () => decidir(s, 'aceptada', el, aceptar));
      descartar.addEventListener('click', () => decidir(s, 'descartada', el, descartar));
      botones.append(descartar, aceptar);
    } else if (puedeIrA(s)) {
      botones.append(h('a', { class: 'btn btn-secundario btn-peq', href: url(rutaDestino(s)), 'data-enlace': '' }, 'Ver'));
    }
    el.append(
      h('div', { class: 'reco-icono' }, icono(ICONO_TIPO[s.tipo] || 'lightbulb', { tam: 20 })),
      h('div', { class: 'crece' },
        h('div', { class: 'fila' }, h('h3', { id: 'reco-' + s.clave.replace(/\W/g, '-') }, s.titulo), insignia(textoPrioridad, tipoPrioridad)),
        h('p', { class: 'motivo' }, s.motivo, s.valor_en_riesgo > 0 ? ` Valor en riesgo: ${pesos(s.valor_en_riesgo)}.` : ''),
        h('p', { class: 'accion' }, icono('arrow-right', { tam: 14 }), s.accion)),
      botones);
    return el;
  }

  async function cargar() {
    const r = await get('/api/sugerencias');
    if (!r.ok) { cuerpo.replaceChildren(errorCarga(r, cargar)); return; }
    todas = r.data;
    pintar();
  }
  cargar();

  return h('section', { class: 'tarjeta recomendaciones ancho-4', 'aria-labelledby': 'titulo-recos' },
    h('div', { class: 'tarjeta-cabecera' },
      h('div', {}, h('h2', { class: 'tarjeta-titulo', id: 'titulo-recos' }, icono('lightbulb', { tam: 22 }), 'Recomendaciones'),
        h('p', { class: 'texto-2 texto-peq', style: { marginTop: '4px' } }, 'Qué conviene hacer hoy según vencimientos y existencias.')),
      h('div', { class: 'fila' }, contador, boton('Actualizar', { peq: true, tipo: 'fantasma', icono: 'refresh-cw', onclick: cargar }))),
    cuerpo);
}

// Tarjeta de métrica con skeleton y estado vacío/error propios.
function metrica({ titulo, icono: ic, tono = '', cargar, enlace }) {
  const valor = h('div', {}, esqueleto({ lineas: 1, alto: 36, ancho: '60%' }));
  const detalle = h('p', { class: 'texto-2 texto-peq' }, esqueleto({ lineas: 1, alto: 12, ancho: '80%' }));
  const tarjeta = h('section', { class: 'tarjeta kpi', 'aria-busy': 'true' },
    h('div', { class: 'fila-entre' }, h('h2', { class: 'kpi-etiqueta' }, titulo), h('span', { class: 'kpi-icono ' + tono }, icono(ic, { tam: 18 }))),
    valor, detalle,
    enlace ? h('a', { class: 'texto-peq negrita', href: url(enlace.ruta), 'data-enlace': '' }, enlace.texto, ' →') : null);
  cargar().then(({ v, d, error }) => {
    tarjeta.removeAttribute('aria-busy');
    if (error) { valor.replaceChildren(h('span', { class: 'texto-peq', style: { color: 'var(--color-error)' } }, 'No disponible')); detalle.textContent = error; return; }
    valor.replaceChildren(h('span', { class: 'kpi-valor' }, v));
    detalle.textContent = d;
  });
  return tarjeta;
}

export default function dashboard(contenedor) {
  const u = usuarioActual();
  const nombre = u.nombre.split(' ')[0];

  const tablero = h('div', { class: 'tablero' });
  if (puede('alertas.ver')) tablero.append(tarjetaRecomendaciones());

  // Las alertas alimentan dos métricas: se piden una sola vez.
  const alertas = puede('alertas.ver') ? Promise.all([get('/api/alertas/vencimientos'), get('/api/alertas/stock-bajo')]) : null;

  if (puede('ventas.ver')) {
    const hoy = hoyISO();
    tablero.append(metrica({
      titulo: puede('ventas.ver_todas') ? 'Ventas de hoy' : 'Mis ventas de hoy', icono: 'shopping-cart',
      enlace: { ruta: '/app/ventas', texto: 'Ver ventas' },
      cargar: async () => {
        const r = await get(`/api/ventas?desde=${hoy}&hasta=${hoy}&limite=500`);
        if (!r.ok) return { error: r.error };
        const total = r.data.reduce((s, v) => s + Number(v.total), 0);
        return { v: pesos(total), d: r.data.length ? `${numero(r.data.length)} venta${r.data.length === 1 ? '' : 's'} registradas hoy` : 'Aún no hay ventas hoy' };
      },
    }));
  }
  if (alertas) {
    tablero.append(metrica({
      titulo: 'Stock bajo', icono: 'package-minus', tono: 'alerta', enlace: { ruta: '/app/alertas', texto: 'Ver alertas' },
      cargar: async () => {
        const [, s] = await alertas;
        if (!s.ok) return { error: s.error };
        return { v: numero(s.data.length), d: s.data.length ? `${s.data.filter((p) => p.stock <= 0).length} agotados` : 'Todo por encima del mínimo' };
      },
    }));
    tablero.append(metrica({
      titulo: 'Lotes por vencer', icono: 'clock', tono: 'error', enlace: { ruta: '/app/alertas', texto: 'Ver vencimientos' },
      cargar: async () => {
        const [v] = await alertas;
        if (!v.ok) return { error: v.error };
        const vencidos = v.data.filter((x) => x.nivel === 'vencido').length;
        const riesgo = v.data.reduce((s, x) => s + Number(x.valor_en_riesgo), 0);
        return { v: numero(v.data.length), d: v.data.length ? `${vencidos} vencidos · ${pesos(riesgo)} en riesgo` : 'Ningún lote en la ventana de alerta' };
      },
    }));
  }
  if (puede('productos.ver')) {
    tablero.append(metrica({
      titulo: 'Productos activos', icono: 'package', enlace: { ruta: '/app/productos', texto: 'Ver catálogo' },
      cargar: async () => {
        const r = await get('/api/productos');
        if (!r.ok) return { error: r.error };
        const sinStock = r.data.filter((p) => p.stock <= 0).length;
        return { v: numero(r.data.length), d: sinStock ? `${sinStock} sin existencias vigentes` : 'Todos con existencias' };
      },
    }));
  }

  const accesos = modulosVisibles(MODULOS).filter((m) => m.id !== 'inicio');
  tablero.append(h('section', { class: 'tarjeta ancho-4', 'aria-labelledby': 'titulo-accesos' },
    h('div', { class: 'tarjeta-cabecera' }, h('h2', { class: 'tarjeta-titulo', id: 'titulo-accesos' }, 'Accesos rápidos')),
    accesos.length
      ? h('div', { class: 'accesos' }, accesos.map((m) => h('a', { class: 'acceso-rapido', href: url(m.ruta), 'data-enlace': '' },
        h('span', { class: 'icono-caja' }, icono(m.icono, { tam: 20 })),
        h('span', { class: 'crece' }, h('span', { class: 'negrita' }, m.titulo), h('span', { class: 'sub', style: { display: 'block' } }, m.descripcion)),
        icono('chevron-right', { tam: 18 }))))
      : vacio({ icono: 'lock', titulo: 'Aún no tiene módulos habilitados', texto: 'Su usuario no tiene roles con permisos. Pídale a un administrador que le asigne un rol.' })));

  contenedor.append(
    h('div', { class: 'cabecera-pagina saludo' }, h('div', {},
      h('h1', {}, `${saludo()}, ${nombre}`),
      h('div', { class: 'meta' }, icono('calendar', { tam: 16 }), h('span', {}, fechaLarga().replace(/^./, (c) => c.toUpperCase())),
        u.roles.map((r) => insignia(r.nombre, 'exito'))))),
    tablero);
}
