// Ventas: registro con carrito (FEFO automático) e historial con detalle.
import { h, pesos, numero, fechaHora, hoyISO, haceDiasISO } from '../nucleo/dom.js';
import { icono } from '../nucleo/iconos.js';
import { get, post } from '../nucleo/api.js';
import { puede } from '../nucleo/sesion.js';
import { tabla, insignia, aviso, vacio, esqueletoTabla, errorCarga, boton, botonIcono, campo, conCarga, notificar, modal, esqueleto, segmentado } from '../nucleo/ui.js';

function nuevaVenta() {
  let productos = [];
  let carrito = [];
  let medio = 'efectivo';
  const selProducto = h('select', { required: true }, h('option', { value: '' }, 'Cargando productos…'));
  const cantidad = h('input', { type: 'number', min: '1', max: '1000000', value: '1', required: true, inputmode: 'numeric' });
  const precio = h('input', { type: 'number', min: '0', step: '0.01', placeholder: 'Precio del catálogo', inputmode: 'decimal' });
  const mensaje = h('div', { 'aria-live': 'polite' });
  const listaCarrito = h('div', {});
  const total = h('span', { class: 'num' }, pesos(0));
  const guardar = boton('Registrar venta', { tipo: 'primario', icono: 'check', class: 'btn-bloque btn-grande' });

  async function cargarProductos() {
    const r = await get('/api/productos');
    if (!r.ok) { mensaje.replaceChildren(aviso(r.error, 'error')); return; }
    productos = r.data;
    selProducto.replaceChildren(h('option', { value: '' }, 'Seleccione un producto'),
      ...productos.map((p) => h('option', { value: p.id, disabled: p.stock <= 0 }, `${p.nombre}${p.presentacion ? ' · ' + p.presentacion : ''} — ${pesos(p.precio_venta)} — ${p.stock > 0 ? 'stock ' + numero(p.stock) : 'agotado'}${p.es_granel ? ' (100 g/u)' : ''}`)));
  }

  function pintarCarrito() {
    const suma = carrito.reduce((s, l) => s + l.cantidad * Number(l.precio_unitario ?? l.producto.precio_venta), 0);
    total.textContent = pesos(suma);
    guardar.disabled = !carrito.length;
    listaCarrito.replaceChildren(carrito.length ? tabla([
      { titulo: 'Producto', valor: (l) => h('span', {}, h('span', { class: 'negrita' }, l.producto.nombre), l.precio_unitario !== undefined ? h('span', { class: 'sub' }, 'Precio de oferta') : null) },
      { titulo: 'Cant.', clase: 'derecha num', valor: (l) => numero(l.cantidad) },
      { titulo: 'Subtotal', clase: 'derecha num', valor: (l) => pesos(l.cantidad * Number(l.precio_unitario ?? l.producto.precio_venta)) },
      { titulo: '', clase: 'derecha', valor: (l) => botonIcono('trash-2', `Quitar ${l.producto.nombre}`, { onclick: () => { carrito = carrito.filter((x) => x !== l); pintarCarrito(); } }) },
    ], carrito, { etiqueta: 'Carrito' }) : vacio({ icono: 'shopping-basket', titulo: 'El carrito está vacío', texto: 'Agregue productos para registrar la venta.' }));
  }

  const agregarBtn = boton('Agregar', { icono: 'plus' });
  agregarBtn.addEventListener('click', () => {
    mensaje.replaceChildren();
    const p = productos.find((x) => String(x.id) === selProducto.value);
    const n = Number(cantidad.value);
    if (!p) { mensaje.replaceChildren(aviso('Seleccione un producto.', 'error')); selProducto.focus(); return; }
    if (!Number.isInteger(n) || n < 1 || n > 1000000) { mensaje.replaceChildren(aviso('La cantidad debe ser un número entero entre 1 y 1.000.000.', 'error')); cantidad.focus(); return; }
    if (n > p.stock) { mensaje.replaceChildren(aviso(`Solo hay ${numero(p.stock)} unidades vigentes de ${p.nombre}.`, 'error')); cantidad.focus(); return; }
    if (carrito.some((l) => l.producto.id === p.id)) { mensaje.replaceChildren(aviso('Ese producto ya está en el carrito; quítelo para cambiar la cantidad.', 'advertencia')); return; }
    const linea = { producto: p, cantidad: n };
    if (precio.value !== '') {
      if (!precio.checkValidity()) { precio.reportValidity(); return; }
      linea.precio_unitario = precio.value;
    }
    carrito.push(linea);
    precio.value = '';
    cantidad.value = '1';
    selProducto.value = '';
    pintarCarrito();
    selProducto.focus();
  });

  guardar.addEventListener('click', async () => {
    const cuerpo = { medio_pago: medio, detalles: carrito.map((l) => ({ producto_id: l.producto.id, cantidad: l.cantidad, ...(l.precio_unitario !== undefined ? { precio_unitario: l.precio_unitario } : {}) })) };
    const r = await conCarga(guardar, post('/api/ventas', cuerpo));
    if (!r.ok) { mensaje.replaceChildren(aviso(r.error, 'error')); return; }
    const lotes = r.data.detalles.flatMap((d) => d.asignacion.map((a) => `#${a.lote_id}: ${a.tomar}`)).join(', ');
    mensaje.replaceChildren(aviso(`Venta #${r.data.id} registrada por ${pesos(r.data.total)}. Lotes usados (FEFO): ${lotes}.`, 'exito'));
    notificar(`Venta #${r.data.id} registrada.`, 'exito');
    carrito = [];
    pintarCarrito();
    cargarProductos();
    document.dispatchEvent(new Event('venta-registrada'));
  });

  pintarCarrito();
  cargarProductos();
  return h('section', { class: 'tarjeta pila', 'aria-labelledby': 't-nueva' },
    h('h3', { id: 't-nueva', class: 'tarjeta-titulo' }, icono('shopping-cart', { tam: 20 }), 'Nueva venta'),
    h('p', { class: 'texto-2 texto-peq' }, 'El sistema toma primero los lotes que vencen antes y excluye los vencidos. En granel, 1 unidad = 100 g.'),
    campo('Producto', selProducto),
    h('div', { class: 'rejilla-form' }, campo('Cantidad', cantidad), campo('Precio de oferta (opcional)', precio, { ayuda: 'Vacío = precio del catálogo' })),
    h('div', {}, agregarBtn),
    mensaje,
    listaCarrito,
    h('div', { class: 'pila-1' }, h('span', { class: 'etiqueta texto-peq negrita' }, 'Medio de pago'),
      segmentado('medio-pago', [{ valor: 'efectivo', texto: 'Efectivo' }, { valor: 'nequi', texto: 'Nequi' }, { valor: 'daviplata', texto: 'Daviplata' }], medio, { etiqueta: 'Medio de pago', alCambiar: (v) => { medio = v; } })),
    h('div', { class: 'carrito-total' }, h('span', {}, 'Total estimado'), total),
    guardar);
}

function historial() {
  const desde = h('input', { type: 'date', value: haceDiasISO(29), required: true });
  const hasta = h('input', { type: 'date', value: hoyISO(), required: true });
  const caja = h('div', {}, esqueletoTabla());
  const LIMITE = 20;
  let offset = 0;
  const info = h('span', {});
  const ant = boton('Anterior', { peq: true, onclick: () => { offset = Math.max(0, offset - LIMITE); cargar(); } });
  const sig = boton('Siguiente', { peq: true, onclick: () => { offset += LIMITE; cargar(); } });

  async function verDetalle(v) {
    const cuerpo = h('div', {}, esqueleto({ lineas: 4 }));
    modal({ titulo: `Venta #${v.id}`, descripcion: `${fechaHora(v.fecha_hora)} · ${v.usuario} · ${v.medio_pago}`, contenido: cuerpo });
    const r = await get('/api/ventas/' + v.id);
    if (!r.ok) { cuerpo.replaceChildren(aviso(r.error, 'error')); return; }
    cuerpo.replaceChildren(tabla([
      { titulo: 'Producto', valor: (d) => d.nombre },
      { titulo: 'Cant.', clase: 'derecha num', valor: (d) => numero(d.cantidad) },
      { titulo: 'Precio', clase: 'derecha num', valor: (d) => pesos(d.precio_unitario) },
      { titulo: 'Subtotal', clase: 'derecha num', valor: (d) => pesos(d.subtotal) },
    ], r.data.detalles, { etiqueta: 'Detalle de la venta' }), h('div', { class: 'carrito-total' }, h('span', {}, 'Total'), h('span', { class: 'num' }, pesos(r.data.total))));
  }

  async function cargar() {
    caja.replaceChildren(esqueletoTabla());
    const r = await get(`/api/ventas?desde=${desde.value}&hasta=${hasta.value}&limite=${LIMITE}&offset=${offset}`);
    if (!r.ok) { caja.replaceChildren(errorCarga(r, cargar)); return; }
    info.textContent = r.data.length ? `${offset + 1}–${offset + r.data.length}` : '';
    ant.disabled = offset === 0;
    sig.disabled = r.data.length < LIMITE;
    caja.replaceChildren(r.data.length ? tabla([
      { titulo: 'Venta', valor: (v) => h('span', {}, h('span', { class: 'negrita' }, '#' + v.id), h('span', { class: 'sub' }, fechaHora(v.fecha_hora))) },
      { titulo: 'Vendedor', valor: (v) => v.usuario },
      { titulo: 'Pago', valor: (v) => insignia(v.medio_pago, 'info') },
      { titulo: 'Total', clase: 'derecha num', valor: (v) => pesos(v.total) },
    ], r.data, { alClic: verDetalle, etiqueta: 'Historial de ventas' }) : vacio({ icono: 'shopping-cart', titulo: 'Sin ventas en el periodo', texto: 'Cambie las fechas para consultar otro periodo.' }),
    h('div', { class: 'paginacion' }, info, h('div', { class: 'fila' }, ant, sig)));
  }
  const consultar = boton('Consultar', { icono: 'search', onclick: () => { offset = 0; cargar(); } });
  document.addEventListener('venta-registrada', () => { if (caja.isConnected) cargar(); });
  cargar();
  return h('section', { class: 'tarjeta tarjeta-sin-relleno', 'aria-labelledby': 't-hist' },
    h('div', { class: 'tarjeta-cabecera' }, h('h3', { id: 't-hist', class: 'tarjeta-titulo' }, icono('clock', { tam: 20 }), puede('ventas.ver_todas') ? 'Historial de ventas' : 'Mis ventas')),
    h('div', { class: 'herramientas', style: { padding: '0 24px' } }, campo('Desde', desde), campo('Hasta', hasta), consultar),
    caja);
}

export default function ventas(contenedor) {
  contenedor.append(
    h('div', { class: 'cabecera-pagina' }, h('div', {}, h('h2', {}, 'Ventas'), h('p', {}, 'Registre ventas y consulte el historial con su detalle.'))),
    h('div', { class: 'columnas-2' },
      puede('ventas.crear') ? nuevaVenta() : null,
      puede('ventas.ver') ? historial() : null));
}
