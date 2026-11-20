// Reportes por periodo: ventas, productos más vendidos, pérdidas y rotación.
import { h, pesos, numero, fechaCO, hoyISO, haceDiasISO, texto } from '../nucleo/dom.js';
import { get } from '../nucleo/api.js';
import { puede } from '../nucleo/sesion.js';
import { tabla, insignia, vacio, esqueletoTabla, errorCarga, boton, campo, selector } from '../nucleo/ui.js';

const ESTADOS = { reponer: ['Reponer', 'error'], sin_movimiento: ['Sin movimiento', 'advertencia'], con_movimiento: ['Con movimiento', 'exito'] };
const ETIQUETAS = {
  dia: 'Día', medio_pago: 'Medio de pago', ventas: 'Ventas', total: 'Total', producto_id: 'ID', nombre: 'Producto', unidades: 'Unidades',
  motivo: 'Motivo', accion_previa: 'Acción previa', valor_perdido: 'Valor perdido', stock_minimo: 'Mínimo', stock: 'Stock',
  promedio_diario: 'Promedio diario', dias_cobertura: 'Días de cobertura', reposicion_sugerida: 'Reposición sugerida', sugerencia_compra: 'Reposición sugerida', estado: 'Estado', ultima_venta: 'Última venta',
};
const MONEDA = new Set(['total', 'valor_perdido']);

const REPORTES = [
  { id: 'ventas', ruta: 'ventas', titulo: 'Ventas por día y medio de pago', permiso: 'reportes.ventas' },
  { id: 'productos', ruta: 'ventas?agrupar=producto', titulo: 'Productos más vendidos', permiso: 'reportes.ventas' },
  { id: 'perdidas', ruta: 'perdidas', titulo: 'Pérdidas y acciones previas', permiso: 'reportes.perdidas' },
  { id: 'rotacion', ruta: 'rotacion', titulo: 'Rotación y reposición', permiso: 'reportes.rotacion' },
];

function celda(clave, valor) {
  if (valor === null || valor === undefined) return '—';
  if (clave === 'estado' && ESTADOS[valor]) return insignia(...ESTADOS[valor]);
  if (MONEDA.has(clave)) return pesos(valor);
  if (clave === 'dia' || clave === 'ultima_venta') return fechaCO(valor);
  if (typeof valor === 'number' || /^-?\d+(\.\d+)?$/.test(String(valor))) return numero(valor);
  return texto(valor).replaceAll('_', ' ');
}

export default function reportes(contenedor) {
  const disponibles = REPORTES.filter((r) => puede(r.permiso));
  const tipo = selector(disponibles.map((r) => ({ valor: r.id, texto: r.titulo })), {});
  const desde = h('input', { type: 'date', value: haceDiasISO(29), required: true });
  const hasta = h('input', { type: 'date', value: hoyISO(), required: true });
  const resumen = h('div', { class: 'tablero', style: { marginBottom: '24px' } });
  const caja = h('div', {});

  async function consultar() {
    const rep = disponibles.find((r) => r.id === tipo.value);
    caja.replaceChildren(esqueletoTabla());
    resumen.replaceChildren();
    const sep = rep.ruta.includes('?') ? '&' : '?';
    const r = await get(`/api/reportes/${rep.ruta}${sep}desde=${desde.value}&hasta=${hasta.value}`);
    if (!r.ok) { caja.replaceChildren(errorCarga(r, consultar)); return; }
    if (!r.data.length) { caja.replaceChildren(vacio({ icono: 'chart-column', titulo: 'Sin datos en el periodo', texto: 'Amplíe el rango de fechas.' })); return; }
    const columnas = Object.keys(r.data[0]).filter((k) => k !== 'producto_id');
    const suma = (k) => r.data.reduce((s, f) => s + Number(f[k] || 0), 0);
    const kpi = (etq, valor) => h('div', { class: 'tarjeta kpi' }, h('span', { class: 'kpi-etiqueta' }, etq), h('span', { class: 'kpi-valor' }, valor));
    if (columnas.includes('total')) resumen.append(kpi('Total vendido', pesos(suma('total'))));
    if (columnas.includes('ventas')) resumen.append(kpi('Ventas', numero(suma('ventas'))));
    if (columnas.includes('unidades')) resumen.append(kpi('Unidades', numero(suma('unidades'))));
    if (columnas.includes('valor_perdido')) resumen.append(kpi('Valor perdido', pesos(suma('valor_perdido'))));
    if (rep.id === 'rotacion') resumen.append(kpi('Para reponer', numero(r.data.filter((f) => f.estado === 'reponer').length)));
    const alineadas = (k) => (MONEDA.has(k) || ['ventas', 'unidades', 'stock', 'stock_minimo', 'promedio_diario', 'dias_cobertura', 'reposicion_sugerida', 'sugerencia_compra'].includes(k) ? 'derecha num' : '');
    caja.replaceChildren(h('div', { class: 'tarjeta tarjeta-sin-relleno' }, tabla(columnas.map((k) => ({ titulo: ETIQUETAS[k] || k.replaceAll('_', ' '), clase: alineadas(k), valor: (f) => celda(k, f[k]) })), r.data, { etiqueta: rep.titulo })));
  }

  contenedor.append(
    h('div', { class: 'cabecera-pagina' }, h('div', {}, h('h2', {}, 'Reportes'), h('p', {}, 'Periodos de hasta 367 días. La reposición se estima a 30 días con las ventas del periodo y el stock vigente.'))),
    h('div', { class: 'tarjeta', style: { marginBottom: '24px' } }, h('div', { class: 'herramientas', style: { marginBottom: 0 } },
      campo('Reporte', tipo, { clase: 'busqueda' }), campo('Desde', desde), campo('Hasta', hasta), boton('Consultar', { tipo: 'primario', icono: 'search', onclick: consultar }))),
    resumen, caja);
  consultar();
}
