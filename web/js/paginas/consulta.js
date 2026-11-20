// Consulta por necesidad del cliente: qué ofrecer evitando contraindicaciones.
import { h, pesos, numero } from '../nucleo/dom.js';
import { icono } from '../nucleo/iconos.js';
import { get } from '../nucleo/api.js';
import { puede } from '../nucleo/sesion.js';
import { aviso, vacio, esqueleto, errorCarga, insignia, interruptorConEtiqueta } from '../nucleo/ui.js';
import { abrirFicha } from './productos.js';

export default async function consulta(contenedor) {
  const necesidades = h('div', { class: 'fila', role: 'group', 'aria-label': 'Necesidad del cliente' }, esqueleto({ lineas: 1, alto: 36 }));
  const contras = h('div', { class: 'fila', role: 'group', 'aria-label': 'Contraindicaciones a evitar' });
  const resultado = h('div', { 'aria-live': 'polite' }, vacio({ icono: 'heart-pulse', titulo: 'Elija una necesidad', texto: 'Seleccione lo que busca el cliente para ver los productos recomendados.' }));
  const evitar = new Set();
  let necesidad = null;
  const { elemento: interruptorDisponibles, sw: disponibles } = interruptorConEtiqueta('Solo con existencias', { activo: true, alCambiar: () => buscar() });

  async function buscar() {
    if (!necesidad) return;
    resultado.replaceChildren(esqueleto({ lineas: 4, alto: 56 }));
    const params = new URLSearchParams({ necesidad });
    if (evitar.size) params.set('evitar', [...evitar].join(','));
    if (disponibles.getAttribute('aria-checked') === 'true') params.set('disponibles', '1');
    const r = await get('/api/recomendaciones?' + params);
    if (!r.ok) { resultado.replaceChildren(errorCarga(r, buscar)); return; }
    const { necesidad: n, evitando, productos, aviso: textoAviso } = r.data;
    resultado.replaceChildren(h('div', { class: 'pila' },
      h('h3', {}, `Para «${n.nombre}»`, evitando.length ? h('span', { class: 'texto-2', style: { fontWeight: 500 } }, ` · evitando ${evitando.map((e) => e.nombre).join(', ')}`) : ''),
      productos.length
        ? h('div', { class: 'accesos' }, productos.map((p) => h('button', {
          type: 'button', class: 'acceso-rapido', style: { textAlign: 'left', cursor: puede('productos.ver') ? 'pointer' : 'default' },
          onclick: () => { if (puede('productos.ver')) abrirFicha(p.id); },
        },
        h('span', { class: 'icono-caja' }, icono('leaf', { tam: 20 })),
        h('span', { class: 'crece' }, h('span', { class: 'negrita', style: { display: 'block' } }, p.nombre),
          h('span', { class: 'sub', style: { display: 'block' } }, `${pesos(p.precio_venta)} · `, p.stock > 0 ? `${numero(p.stock)} disponibles` : 'Agotado')),
        p.stock > 0 ? insignia('Disponible', 'exito') : insignia('Agotado', 'error'))))
        : vacio({ icono: 'search', titulo: 'No hay productos que cumplan los filtros', texto: 'Quite alguna contraindicación o desactive «Solo con existencias».' }),
      aviso(textoAviso, 'advertencia')));
  }

  contenedor.append(
    h('div', { class: 'cabecera-pagina' }, h('div', {}, h('h2', {}, 'Consulta por necesidad'), h('p', {}, 'Información orientativa del catálogo: los productos son complementos, no medicamentos.'))),
    h('div', { class: 'tarjeta pila-3' },
      h('div', { class: 'pila-1' }, h('h3', {}, '1. ¿Qué necesita el cliente?'), necesidades),
      h('div', { class: 'pila-1' }, h('h3', {}, '2. ¿Debe evitar algo? (opcional)'), contras),
      interruptorDisponibles),
    h('div', { class: 'tarjeta', style: { marginTop: '24px' } }, resultado));

  const [rn, rc] = await Promise.all([get('/api/etiquetas?tipo=necesidad'), get('/api/etiquetas?tipo=contraindicacion')]);
  if (!rn.ok || !rc.ok) { necesidades.replaceChildren(errorCarga(rn.ok ? rc : rn)); return; }
  if (!rn.data.length) necesidades.replaceChildren(h('p', { class: 'texto-2' }, 'No hay necesidades registradas en el catálogo.'));
  else necesidades.replaceChildren(...rn.data.map((e) => h('button', {
    type: 'button', class: 'chip', 'aria-pressed': 'false',
    onclick: (ev) => {
      necesidades.querySelectorAll('.chip').forEach((c) => c.setAttribute('aria-pressed', 'false'));
      ev.currentTarget.setAttribute('aria-pressed', 'true');
      necesidad = e.id;
      buscar();
    },
  }, e.nombre, h('span', { class: 'texto-mini', style: { opacity: .75 } }, `(${e.cantidad_productos})`))));
  contras.replaceChildren(...(rc.data.length ? rc.data.map((e) => h('button', {
    type: 'button', class: 'chip', 'aria-pressed': 'false',
    onclick: (ev) => {
      const activo = ev.currentTarget.getAttribute('aria-pressed') !== 'true';
      ev.currentTarget.setAttribute('aria-pressed', String(activo));
      if (activo) evitar.add(e.id); else evitar.delete(e.id);
      buscar();
    },
  }, icono('ban', { tam: 14 }), e.nombre)) : [h('p', { class: 'texto-2' }, 'No hay contraindicaciones registradas.')]));
}
