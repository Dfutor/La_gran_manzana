// Alertas: vencimientos por nivel y productos con stock bajo.
import { h, pesos, numero, texto, fechaCO } from '../nucleo/dom.js';
import { get } from '../nucleo/api.js';
import { tabla, insignia, vacio, esqueletoTabla, errorCarga, boton } from '../nucleo/ui.js';

const NIVELES = { vencido: ['Vencido', 'error'], liquidar: ['Liquidar', 'advertencia'], devolver: ['Devolver', 'info'] };

export default function alertas(contenedor) {
  const cajaVenc = h('div', {}, esqueletoTabla());
  const cajaStock = h('div', {}, esqueletoTabla());
  const filtros = h('div', { class: 'fila', role: 'group', 'aria-label': 'Filtrar por nivel' });
  let filtro = '';
  let vencimientos = [];

  function pintarVenc() {
    const lista = filtro ? vencimientos.filter((v) => v.nivel === filtro) : vencimientos;
    cajaVenc.replaceChildren(lista.length ? tabla([
      { titulo: 'Nivel', valor: (v) => insignia(...NIVELES[v.nivel]) },
      { titulo: 'Producto', valor: (v) => h('span', {}, h('span', { class: 'negrita' }, v.producto), h('span', { class: 'sub' }, `Lote ${texto(v.codigo_lote || '#' + v.lote_id)}`)) },
      { titulo: 'Vence', valor: (v) => h('span', {}, fechaCO(v.fecha_vencimiento), h('span', { class: 'sub' }, v.dias_restantes < 0 ? `hace ${-v.dias_restantes} días` : `en ${v.dias_restantes} días`)) },
      { titulo: 'Cant.', clase: 'derecha num', valor: (v) => numero(v.cantidad_actual) },
      { titulo: 'En riesgo', clase: 'derecha num', valor: (v) => pesos(v.valor_en_riesgo) },
      { titulo: 'Acción sugerida', valor: (v) => v.accion_sugerida },
    ], lista, { etiqueta: 'Lotes en alerta de vencimiento' }) : vacio({ icono: 'circle-check', titulo: 'Sin lotes en esta categoría', texto: 'Ningún lote está dentro de la ventana de alerta.' }));
  }

  async function cargar() {
    cajaVenc.replaceChildren(esqueletoTabla());
    cajaStock.replaceChildren(esqueletoTabla());
    const [v, s] = await Promise.all([get('/api/alertas/vencimientos'), get('/api/alertas/stock-bajo')]);
    if (!v.ok) cajaVenc.replaceChildren(errorCarga(v, cargar));
    else {
      vencimientos = v.data;
      const cuenta = (n) => vencimientos.filter((x) => x.nivel === n).length;
      filtros.replaceChildren(...[['', `Todos (${vencimientos.length})`], ...Object.entries(NIVELES).map(([k, [t]]) => [k, `${t} (${cuenta(k)})`])].map(([valor, etq]) =>
        h('button', { type: 'button', class: 'chip', 'aria-pressed': String(filtro === valor), onclick: (e) => { filtro = valor; filtros.querySelectorAll('.chip').forEach((c) => c.setAttribute('aria-pressed', 'false')); e.currentTarget.setAttribute('aria-pressed', 'true'); pintarVenc(); } }, etq)));
      pintarVenc();
    }
    if (!s.ok) cajaStock.replaceChildren(errorCarga(s, cargar));
    else cajaStock.replaceChildren(s.data.length ? tabla([
      { titulo: 'Producto', valor: (p) => h('span', { class: 'negrita' }, p.nombre) },
      { titulo: 'Categoría', valor: (p) => p.categoria },
      { titulo: 'Stock', clase: 'derecha num', valor: (p) => p.stock <= 0 ? insignia('Agotado', 'error') : numero(p.stock) },
      { titulo: 'Mínimo', clase: 'derecha num', valor: (p) => numero(p.stock_minimo) },
      { titulo: 'Faltan', clase: 'derecha num', valor: (p) => numero(p.faltante_para_minimo) },
    ], s.data, { etiqueta: 'Productos con stock bajo' }) : vacio({ icono: 'circle-check', titulo: 'Todo por encima del mínimo' }));
  }

  contenedor.append(
    h('div', { class: 'cabecera-pagina' }, h('div', {}, h('h2', {}, 'Alertas'), h('p', {}, 'Se recalculan en cada consulta; el servicio también las revisa cada minuto.')),
      boton('Actualizar', { icono: 'refresh-cw', onclick: cargar })),
    h('section', { class: 'tarjeta tarjeta-sin-relleno', 'aria-labelledby': 't-venc' },
      h('div', { class: 'tarjeta-cabecera' }, h('h3', { id: 't-venc' }, 'Vencimientos'), filtros), cajaVenc),
    h('section', { class: 'tarjeta tarjeta-sin-relleno', style: { marginTop: '24px' }, 'aria-labelledby': 't-stock' },
      h('div', { class: 'tarjeta-cabecera' }, h('h3', { id: 't-stock' }, 'Stock bajo')), cajaStock));
  cargar();
}
