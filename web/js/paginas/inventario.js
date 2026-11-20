// Lotes y bajas: ingreso de lotes, consulta y corrección, registro e historial de bajas.
// Acepta ?ingreso=PRODUCTO y ?baja=LOTE&motivo=... (desde las recomendaciones del panel).
import { h, pesos, numero, texto, fechaCO, fechaHora, hoyISO, haceDiasISO } from '../nucleo/dom.js';
import { get, post, put } from '../nucleo/api.js';
import { puede } from '../nucleo/sesion.js';
import { tabla, insignia, aviso, vacio, esqueletoTabla, errorCarga, boton, botonIcono, campo, conCarga, notificar, pestanas, modalFormulario, selector } from '../nucleo/ui.js';

const MOTIVOS = [['vencimiento', 'Vencimiento'], ['deterioro', 'Deterioro'], ['obsequio', 'Obsequio'], ['devolucion', 'Devolución al distribuidor'], ['otro', 'Otro']];
const ACCIONES = [['ninguna', 'Ninguna'], ['venta_destacada', 'Venta destacada'], ['rebaja', 'Rebaja'], ['combo', 'Combo'], ['devolucion_distribuidor', 'Devolución al distribuidor'], ['obsequio', 'Obsequio']];
const opciones = (lista, valor) => lista.map(([v, t]) => ({ valor: v, texto: t, seleccionado: v === valor }));

let productosCache = null;
async function productos() {
  productosCache ??= get('/api/productos').then((r) => (r.ok ? r.data : []));
  return productosCache;
}
async function selectorProductos(valor) {
  const lista = await productos();
  return selector([{ valor: '', texto: 'Seleccione un producto' }, ...lista.map((p) => ({ valor: p.id, texto: `${p.nombre}${p.presentacion ? ' · ' + p.presentacion : ''}${p.es_granel ? ' (100 g/u)' : ''}`, seleccionado: String(p.id) === String(valor) }))], { required: true });
}

function formulario(campos, textoBoton, alEnviar) {
  const mensaje = h('div', { 'aria-live': 'polite' });
  const enviar = boton(textoBoton, { tipo: 'primario', submit: true, icono: 'check' });
  const form = h('form', { class: 'pila', novalidate: true }, mensaje, h('div', { class: 'rejilla-form' }, campos), h('div', {}, enviar));
  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    mensaje.replaceChildren();
    if (!form.checkValidity()) { form.reportValidity(); return; }
    const r = await conCarga(enviar, alEnviar(form));
    mensaje.replaceChildren(aviso(r.texto, r.ok ? 'exito' : 'error'));
    if (r.ok) { notificar(r.texto, 'exito'); form.reset(); }
  });
  return form;
}

async function panelIngreso(productoInicial) {
  const sel = await selectorProductos(productoInicial);
  sel.name = 'producto_id';
  return h('div', { class: 'pila' },
    h('p', { class: 'texto-2' }, 'Registre cada compra con su factura y vencimiento. No se aceptan lotes ya vencidos.'),
    formulario([
      campo('Producto', sel, { clase: 'ancho-total' }),
      campo('Código del lote', h('input', { name: 'codigo_lote', maxlength: '60' })),
      campo('Número de factura', h('input', { name: 'numero_factura', maxlength: '50' })),
      campo('Fecha de vencimiento', h('input', { name: 'fecha_vencimiento', type: 'date', required: true, min: hoyISO() })),
      campo('Cantidad', h('input', { name: 'cantidad', type: 'number', min: '1', max: '1000000', required: true, inputmode: 'numeric' }), { ayuda: 'Granel: unidades de 100 g' }),
      campo('Costo por unidad', h('input', { name: 'costo_unitario', type: 'number', min: '0', step: '0.01', required: true, inputmode: 'decimal' })),
    ], 'Guardar ingreso', async (f) => {
      const d = Object.fromEntries(new FormData(f));
      const r = await post('/api/lotes', { producto_id: Number(d.producto_id), codigo_lote: d.codigo_lote || null, numero_factura: d.numero_factura || null, fecha_vencimiento: d.fecha_vencimiento, cantidad: Number(d.cantidad), costo_unitario: String(d.costo_unitario) });
      productosCache = null;
      return { ok: r.ok, texto: r.ok ? `Lote #${r.data.id} ingresado con ${numero(r.data.cantidad_actual)} unidades.` : r.error };
    }));
}

async function panelLotes() {
  const sel = await selectorProductos('');
  sel.required = false;
  sel.options[0].textContent = 'Todos los productos';
  const caja = h('div', {}, vacio({ icono: 'boxes', titulo: 'Seleccione un producto', texto: 'O consulte todos los lotes.' }));
  function editar(l) {
    modalFormulario({
      titulo: `Corregir lote #${l.id}`, descripcion: `${l.producto}. Las cantidades cambian solo con ingresos, ventas y bajas.`,
      campos: h('div', { class: 'rejilla-form' },
        campo('Código del lote', h('input', { name: 'codigo_lote', maxlength: '60', value: l.codigo_lote || '' })),
        campo('Número de factura', h('input', { name: 'numero_factura', maxlength: '50', value: l.numero_factura || '' })),
        campo('Fecha de vencimiento', h('input', { name: 'fecha_vencimiento', type: 'date', required: true, value: String(l.fecha_vencimiento).slice(0, 10) }))),
      enviar: async (f) => {
        const d = Object.fromEntries(new FormData(f));
        const r = await put('/api/lotes/' + l.id, { codigo_lote: d.codigo_lote || null, numero_factura: d.numero_factura || null, fecha_vencimiento: d.fecha_vencimiento });
        if (r.ok) { notificar(`Lote #${l.id} actualizado.`, 'exito'); cargar(); }
        return r;
      },
    });
  }
  async function cargar() {
    caja.replaceChildren(esqueletoTabla());
    const r = await get(`/api/lotes?limite=500${sel.value ? '&producto_id=' + sel.value : ''}`);
    if (!r.ok) { caja.replaceChildren(errorCarga(r, cargar)); return; }
    const columnas = [
      { titulo: 'Lote', valor: (l) => h('span', {}, h('span', { class: 'negrita' }, texto(l.codigo_lote || '#' + l.id)), h('span', { class: 'sub' }, `#${l.id} · factura ${texto(l.numero_factura)}`)) },
      { titulo: 'Producto', valor: (l) => l.producto },
      { titulo: 'Vence', valor: (l) => h('span', {}, fechaCO(l.fecha_vencimiento), ' ', Number(l.dias_restantes) < 0 ? insignia('Vencido', 'error') : null) },
      { titulo: 'Actual / inicial', clase: 'derecha num', valor: (l) => `${numero(l.cantidad_actual)} / ${numero(l.cantidad_inicial)}` },
      { titulo: 'Costo', clase: 'derecha num', valor: (l) => pesos(l.costo_unitario) },
    ];
    if (puede('lotes.editar')) columnas.push({ titulo: '', clase: 'derecha', valor: (l) => botonIcono('pencil', `Corregir lote #${l.id}`, { onclick: () => editar(l) }) });
    caja.replaceChildren(r.data.length ? tabla(columnas, r.data, { etiqueta: 'Lotes' }) : vacio({ icono: 'boxes', titulo: 'No hay lotes registrados' }));
  }
  sel.addEventListener('change', cargar);
  cargar();
  return h('div', { class: 'pila' }, h('div', { class: 'herramientas' }, campo('Producto', sel, { clase: 'busqueda' })), h('div', { class: 'tarjeta tarjeta-sin-relleno' }, caja));
}

function panelBaja(loteInicial, motivoInicial) {
  return h('div', { class: 'pila' },
    h('p', { class: 'texto-2' }, 'La baja descuenta unidades del lote y registra la pérdida al costo. Una baja por vencimiento exige que el lote ya esté vencido.'),
    formulario([
      campo('ID del lote', h('input', { name: 'lote_id', type: 'number', min: '1', required: true, value: loteInicial || '', inputmode: 'numeric' }), { ayuda: 'Aparece en «Lotes» y en las alertas' }),
      campo('Cantidad', h('input', { name: 'cantidad', type: 'number', min: '1', max: '1000000', required: true, inputmode: 'numeric' })),
      campo('Motivo', selector(opciones(MOTIVOS, motivoInicial || 'vencimiento'), { name: 'motivo' })),
      campo('Acción previa', selector(opciones(ACCIONES, motivoInicial === 'devolucion' ? 'devolucion_distribuidor' : 'ninguna'), { name: 'accion_previa' }), { ayuda: 'Qué se intentó antes de dar de baja' }),
    ], 'Registrar baja', async (f) => {
      const d = Object.fromEntries(new FormData(f));
      const r = await post('/api/bajas', { lote_id: Number(d.lote_id), cantidad: Number(d.cantidad), motivo: d.motivo, accion_previa: d.accion_previa });
      productosCache = null;
      return { ok: r.ok, texto: r.ok ? `Baja #${r.data.id} registrada: ${pesos(r.data.valor_perdido)} perdidos; quedan ${numero(r.data.cantidad_actual)} unidades en el lote.` : r.error };
    }));
}

function panelHistorialBajas() {
  const desde = h('input', { type: 'date', value: haceDiasISO(29) });
  const hasta = h('input', { type: 'date', value: hoyISO() });
  const caja = h('div', {}, esqueletoTabla());
  async function cargar() {
    caja.replaceChildren(esqueletoTabla());
    const r = await get(`/api/bajas?desde=${desde.value}&hasta=${hasta.value}&limite=500`);
    if (!r.ok) { caja.replaceChildren(errorCarga(r, cargar)); return; }
    const motivo = Object.fromEntries(MOTIVOS);
    caja.replaceChildren(r.data.length ? tabla([
      { titulo: 'Baja', valor: (b) => h('span', {}, h('span', { class: 'negrita' }, '#' + b.id), h('span', { class: 'sub' }, fechaHora(b.fecha_baja))) },
      { titulo: 'Producto', valor: (b) => h('span', {}, b.producto, h('span', { class: 'sub' }, `Lote #${texto(b.lote_id)}`)) },
      { titulo: 'Motivo', valor: (b) => insignia(motivo[b.motivo] || b.motivo, b.motivo === 'vencimiento' ? 'error' : 'advertencia') },
      { titulo: 'Cant.', clase: 'derecha num', valor: (b) => numero(b.cantidad) },
      { titulo: 'Pérdida', clase: 'derecha num', valor: (b) => pesos(b.valor_perdido) },
    ], r.data, { etiqueta: 'Historial de bajas' }) : vacio({ icono: 'archive', titulo: 'Sin bajas en el periodo' }));
  }
  cargar();
  return h('div', { class: 'pila' },
    h('div', { class: 'herramientas' }, campo('Desde', desde), campo('Hasta', hasta), boton('Consultar', { icono: 'search', onclick: cargar })),
    h('div', { class: 'tarjeta tarjeta-sin-relleno' }, caja));
}

export default async function inventario(contenedor, { query }) {
  productosCache = null;
  const ingreso = query.get('ingreso');
  const baja = query.get('baja');
  const lista = [
    puede('lotes.crear') && { id: 'ingreso', titulo: 'Ingreso de lote', icono: 'package-plus', crear: () => panelIngreso(ingreso) },
    puede('lotes.ver') && { id: 'lotes', titulo: 'Lotes', icono: 'boxes', crear: panelLotes },
    puede('bajas.crear') && { id: 'baja', titulo: 'Registrar baja', icono: 'package-minus', crear: () => panelBaja(baja, query.get('motivo')) },
    puede('bajas.ver') && { id: 'bajas', titulo: 'Historial de bajas', icono: 'archive', crear: panelHistorialBajas },
  ].filter(Boolean);
  const inicial = (baja && lista.find((p) => p.id === 'baja')) || (ingreso && lista.find((p) => p.id === 'ingreso')) || lista[0];
  const area = h('div', { role: 'tabpanel' });
  async function mostrar(id) {
    const p = lista.find((x) => x.id === id);
    area.setAttribute('aria-labelledby', 'tab-' + id);
    area.replaceChildren(await p.crear());
  }
  const { barra } = pestanas(lista, { activa: inicial.id, alCambiar: mostrar });
  contenedor.append(
    h('div', { class: 'cabecera-pagina' }, h('div', {}, h('h2', {}, 'Lotes y bajas'), h('p', {}, 'Ingresos, correcciones y salidas del inventario por lote.'))),
    h('div', { class: 'tarjeta' }, barra, area));
  if (baja || ingreso) contenedor.prepend(aviso(baja ? `Recomendación aceptada: registre la baja del lote #${baja}.` : 'Recomendación aceptada: registre el ingreso del lote para reponer el producto.', 'info'));
  await mostrar(inicial.id);
}
