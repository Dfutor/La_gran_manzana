// Productos: búsqueda, ficha con lotes FEFO y simulación de venta.
import { h, pesos, numero, texto, fechaCO, debounce } from '../nucleo/dom.js';
import { icono } from '../nucleo/iconos.js';
import { get } from '../nucleo/api.js';
import { url, rutaActual } from '../nucleo/router.js';
import { modal, tabla, insignia, aviso, vacio, esqueletoTabla, errorCarga, esqueleto, campo, boton, conCarga, paginador } from '../nucleo/ui.js';

// Ficha del producto en un panel lateral. Exportada para reutilizarla.
export async function abrirFicha(id) {
  const cuerpo = h('div', { class: 'pila' }, esqueleto({ lineas: 6 }));
  const panel = modal({ titulo: 'Ficha del producto', contenido: cuerpo, panel: true });
  const r = await get('/api/productos/' + encodeURIComponent(id));
  if (!r.ok) { cuerpo.replaceChildren(aviso(r.error, 'error')); return panel; }
  const { ficha: f, etiquetas, lotes } = r.data;
  panel.dialogo.querySelector('h2').textContent = f.nombre + (f.presentacion ? ' · ' + f.presentacion : '');
  const seccion = (titulo, valor) => (valor ? h('div', { class: 'ficha-seccion' }, h('h4', {}, titulo), h('p', { style: { whiteSpace: 'pre-line' } }, valor)) : null);
  const dato = (etq, valor) => h('div', { class: 'ficha-dato' }, h('div', { class: 'etiqueta' }, etq), h('div', { class: 'valor' }, valor));

  // Simulación FEFO: no modifica el inventario.
  const cantidad = h('input', { type: 'number', min: '1', max: '1000000', value: '1', inputmode: 'numeric' });
  const resultado = h('div', { 'aria-live': 'polite' });
  const simular = boton('Simular venta', { peq: true, icono: 'boxes' });
  simular.addEventListener('click', async () => {
    const rp = await conCarga(simular, get(`/api/productos/${id}/plan-fefo?cantidad=${encodeURIComponent(cantidad.value)}`));
    if (!rp.ok) { resultado.replaceChildren(aviso(rp.error, rp.status === 409 ? 'advertencia' : 'error')); return; }
    resultado.replaceChildren(tabla([
      { titulo: 'Lote', valor: (a) => texto(a.codigo_lote || '#' + a.lote_id) },
      { titulo: 'Vence', valor: (a) => fechaCO(a.fecha_vencimiento) },
      { titulo: 'Tomar', clase: 'derecha num', valor: (a) => numero(a.tomar) },
      { titulo: 'Quedaría', clase: 'derecha num', valor: (a) => numero(a.quedaria) },
    ], rp.data.asignacion, { etiqueta: 'Plan FEFO simulado' }));
  });

  cuerpo.replaceChildren(
    aviso(f.aviso_complemento, 'advertencia'),
    h('div', { class: 'ficha-datos' },
      dato('Precio', pesos(f.precio_venta)),
      dato('Stock vigente', h('span', {}, numero(f.stock), ' ', f.stock_bajo ? insignia('Bajo', 'error') : null)),
      dato('Mínimo', numero(f.stock_minimo)),
      dato('Categoría', f.categoria),
      dato('Marca', texto(f.marca)),
      dato('Proveedor', f.proveedor ? f.proveedor.nombre : '—')),
    f.stock_vencido ? aviso(`Hay ${numero(f.stock_vencido)} unidades vencidas en estante: regístrelas como baja.`, 'error') : null,
    etiquetas.length ? h('div', { class: 'fila' }, etiquetas.map((e) => insignia(e.nombre, e.tipo === 'contraindicacion' ? 'error' : e.tipo === 'necesidad' ? 'exito' : 'info'))) : null,
    seccion('Para qué sirve', f.para_que_sirve),
    seccion('Modo de uso', f.modo_toma),
    seccion('Contraindicaciones', f.contraindicaciones),
    seccion('Ingredientes', f.ingredientes),
    h('div', { class: 'ficha-seccion pila-1' }, h('h4', {}, 'Lotes en orden FEFO'), h('p', { class: 'texto-2 texto-peq' }, 'Se vende primero el de arriba (el que vence antes).'),
      lotes.length ? tabla([
        { titulo: 'Lote', valor: (l) => texto(l.codigo_lote || '#' + l.id) },
        { titulo: 'Vence', valor: (l) => fechaCO(l.fecha_vencimiento) },
        { titulo: 'Días', clase: 'derecha num', valor: (l) => l.dias_restantes },
        { titulo: 'Cantidad', clase: 'derecha num', valor: (l) => numero(l.cantidad_actual) },
      ], lotes, { etiqueta: 'Lotes vendibles' }) : aviso('No hay lotes vendibles.', 'advertencia')),
    h('div', { class: 'ficha-seccion pila-1' }, h('h4', {}, 'Simular una venta'),
      h('div', { class: 'fila', style: { alignItems: 'flex-end' } }, campo('Cantidad', cantidad), simular), resultado));
  return panel;
}

export default function productos(contenedor, { query }) {
  const buscar = h('input', { type: 'search', placeholder: 'Nombre, marca o código de barras', maxlength: '100', 'aria-label': 'Buscar productos' });
  const soloBajo = h('input', { type: 'checkbox' });
  const resultados = h('div', {}, esqueletoTabla());
  const pag = paginador(15, () => pintar());
  let datos = [];

  function pintar() {
    const lista = soloBajo.checked ? datos.filter((p) => p.stock_bajo) : datos;
    if (!lista.length) {
      resultados.replaceChildren(vacio({ icono: 'search', titulo: 'Sin resultados', texto: buscar.value ? 'Pruebe con otro nombre, marca o código.' : 'No hay productos activos.' }));
      return;
    }
    resultados.replaceChildren(tabla([
      { titulo: 'Producto', valor: (p) => h('span', {}, h('span', { class: 'negrita' }, p.nombre), ' ', p.es_granel ? insignia('Granel 100 g', 'info') : null, h('span', { class: 'sub' }, [p.marca, p.presentacion].filter(Boolean).join(' · ') || '—')) },
      { titulo: 'Categoría', valor: (p) => p.categoria },
      { titulo: 'Precio', clase: 'derecha num', valor: (p) => pesos(p.precio_venta) },
      { titulo: 'Stock', clase: 'derecha', valor: (p) => h('span', { class: 'fila', style: { justifyContent: 'flex-end' } }, h('span', { class: 'num' }, numero(p.stock)), p.stock <= 0 ? insignia('Agotado', 'error') : p.stock_bajo ? insignia('Bajo', 'advertencia') : null) },
    ], pag.pagina(lista), { alClic: (p) => abrirFicha(p.id), etiqueta: 'Productos' }), pag.pie);
  }

  async function cargar() {
    resultados.replaceChildren(esqueletoTabla());
    const r = await get('/api/productos?q=' + encodeURIComponent(buscar.value.trim()));
    if (!r.ok) { resultados.replaceChildren(errorCarga(r, cargar)); return; }
    datos = r.data;
    pag.reiniciar();
    pintar();
  }
  buscar.addEventListener('input', debounce(cargar, 300));
  soloBajo.addEventListener('change', () => { pag.reiniciar(); pintar(); });

  contenedor.append(
    h('div', { class: 'cabecera-pagina' }, h('div', {}, h('h2', {}, 'Productos'), h('p', {}, 'Busque un producto y ábralo para ver su ficha, sus lotes y simular una venta.'))),
    h('div', { class: 'tarjeta tarjeta-sin-relleno' },
      h('div', { class: 'herramientas', style: { padding: '16px 16px 0' } },
        h('div', { class: 'campo busqueda' }, h('div', { class: 'entrada-icono' }, icono('search', { tam: 18 }), buscar)),
        h('label', { class: 'check' }, soloBajo, 'Solo stock bajo')),
      resultados));
  cargar();
  const id = query.get('id');
  // ?id= abre la ficha (por ejemplo, desde una recomendación); al cerrarla se limpia la URL sin recargar.
  if (id) abrirFicha(id).then((p) => p.dialogo.addEventListener('close', () => { if (rutaActual() === '/app/productos') history.replaceState(null, '', url('/app/productos')); }));
}
