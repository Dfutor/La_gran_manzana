'use strict';

let usuarioSesion = null;
let carrito = [];
let productosVenta = [];

function mostrarSesion(u) {
  usuarioSesion = u;
  $('#form-login').hidden = !!u;
  $('#sesion').hidden = !u;
  $('#aplicacion').hidden = !u;
  $('#sesion-nombre').textContent = u ? `${u.nombre} (${u.rol})` : '';
  document.querySelectorAll('[data-propietario]').forEach(el => { el.hidden = !u || u.rol !== 'propietario'; });
  if (!u) {
    carrito = [];
    $('#carrito').replaceChildren();
    $('#ficha').replaceChildren();
    $('#reporte-lista').replaceChildren();
    $('#formularios-admin').replaceChildren();
    $('#aviso-alertas').textContent = '';
    Object.keys(cargadas).forEach(k => delete cargadas[k]);
  }
}

async function iniciarOperacion() {
  $('#form-login').addEventListener('submit', async e => {
    e.preventDefault();
    const f = e.currentTarget;
    const boton = f.querySelector('button');
    boton.disabled = true;
    const r = await api('/api/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(Object.fromEntries(new FormData(f))) });
    f.elements.password.value = '';
    boton.disabled = false;
    $('#login-msg').textContent = r.ok ? '' : mensajeError(r);
    if (r.ok) entrar(r.data);
  });
  $('#salir').addEventListener('click', async () => {
    const r = await api('/api/logout', { method: 'POST' });
    if (r.ok) { mostrarSesion(null); $('#login-msg').textContent = 'Sesión cerrada.'; }
    else $('#aviso-alertas').textContent = mensajeError(r);
  });
  $('#agregar-venta').addEventListener('click', agregarVenta);
  $('#form-venta').addEventListener('submit', guardarVenta);
  $('#historial-ventas').addEventListener('click', () => cargarTabla('/api/ventas', $('#ventas-lista')));
  $('#historial-bajas').addEventListener('click', () => cargarTabla('/api/bajas', $('#bajas-lista')));
  enlazarFormulario($('#form-lote'), () => '/api/lotes', 'POST', ['producto_id', 'cantidad'], ['costo_unitario']);
  enlazarFormulario($('#form-baja'), () => '/api/bajas', 'POST', ['lote_id', 'cantidad']);
  enlazarFormulario($('#form-editar-lote'), f => '/api/lotes/' + f.elements.id.value, 'PUT');
  $('#form-lotes-buscar').addEventListener('submit', e => {
    e.preventDefault(); cargarTabla('/api/lotes?producto_id=' + $('#lotes-producto').value + '&limite=500', $('#lotes-lista'));
  });
  $('#form-reporte').addEventListener('submit', e => {
    e.preventDefault(); const f = e.currentTarget;
    const tipo = f.elements.tipo.value;
    cargarTabla('/api/reportes/' + tipo + (tipo.includes('?') ? '&' : '?') + new URLSearchParams({ desde: f.elements.desde.value, hasta: f.elements.hasta.value }), $('#reporte-lista'));
  });
  const r = await api('/api/sesion');
  if (r.ok) entrar(r.data);
  setInterval(() => { if (usuarioSesion) actualizarAvisos(); }, 30000);
}
function entrar(u) {
  mostrarSesion(u);
  Object.keys(cargadas).forEach(k => delete cargadas[k]);
  // Cada acceso comienza en el catálogo; las pestañas del propietario se controlan también en el servidor.
  activarPestana('productos');
  verificarSalud();
  actualizarAvisos();
}
async function actualizarAvisos() {
  const [v, s] = await Promise.all([api('/api/alertas/vencimientos'), api('/api/alertas/stock-bajo')]);
  if (!usuarioSesion) return;
  $('#aviso-alertas').textContent = v.ok && s.ok
    ? `Alertas automáticas: ${s.data.length} productos con stock bajo; ${v.data.filter(x => x.nivel === 'vencido').length} lotes vencidos; ${v.data.filter(x => x.nivel !== 'vencido').length} lotes próximos a vencer. Consulte la pestaña Alertas.`
    : 'No se pudieron actualizar las alertas: ' + mensajeError(v.ok ? s : v);
  if (!$('#tab-alertas').hidden) cargarAlertas();
}
async function cargarSelectores() {
  const r = await api('/api/productos');
  if (!r.ok) { $('#venta-msg').textContent = mensajeError(r); return; }
  productosVenta = r.data;
  document.querySelectorAll('.selector-producto, #venta-producto').forEach(s => {
    const anterior = s.value;
    s.replaceChildren(...r.data.map(p => h('option', { value: p.id }, `#${p.id} ${p.nombre} — ${pesos(p.precio_venta)} — stock ${p.stock}${p.es_granel ? ' (100 g/unidad)' : ''}`)));
    if ([...s.options].some(o => o.value === anterior)) s.value = anterior;
  });
}
function agregarVenta() {
  const id = Number($('#venta-producto').value), cantidad = Number($('#venta-cantidad').value);
  if (!id || !Number.isInteger(cantidad) || cantidad < 1 || cantidad > 1000000) { $('#venta-msg').textContent = 'Seleccione un producto y una cantidad válida.'; return; }
  if (carrito.some(l => l.producto_id === id)) { $('#venta-msg').textContent = 'El producto ya está en el carrito; elimínelo para cambiar la cantidad.'; return; }
  const linea = { producto_id: id, cantidad };
  const precio = $('#venta-precio').value;
  if (precio !== '') { if (!$('#venta-precio').checkValidity()) { $('#venta-precio').reportValidity(); return; } linea.precio_unitario = precio; }
  carrito.push(linea); $('#venta-msg').textContent = ''; mostrarCarrito();
}
function mostrarCarrito() {
  $('#carrito').replaceChildren(...carrito.map((l, i) => h('li', {},
    `${productosVenta.find(p => p.id === l.producto_id)?.nombre || l.producto_id}: ${l.cantidad} unidades; precio ${l.precio_unitario === undefined ? 'vigente del catálogo' : pesos(l.precio_unitario)} `,
    h('button', { type: 'button', onclick: () => { carrito.splice(i, 1); mostrarCarrito(); } }, 'Quitar'))));
}
async function guardarVenta(e) {
  e.preventDefault();
  if (!carrito.length) { $('#venta-msg').textContent = 'Agregue productos al carrito.'; return; }
  const boton = $('#guardar-venta'); boton.disabled = true;
  const r = await api('/api/ventas', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ medio_pago: $('#medio-pago').value, detalles: carrito }) });
  boton.disabled = false;
  $('#venta-msg').textContent = r.ok ? `Venta #${r.data.id} guardada. Total ${pesos(r.data.total)}. Lotes: ${r.data.detalles.flatMap(d => d.asignacion.map(a => `#${a.lote_id}: ${a.tomar}`)).join(', ')}` : mensajeError(r);
  if (r.ok) { carrito = []; mostrarCarrito(); cargarSelectores(); actualizarAvisos(); }
}
function enlazarFormulario(form, ruta, metodo, enteros = [], dinero = []) {
  form.addEventListener('submit', async e => {
    e.preventDefault();
    const datos = Object.fromEntries(new FormData(form));
    delete datos.id;
    for (const k of enteros) datos[k] = Number(datos[k]);
    for (const k of dinero) datos[k] = String(datos[k]);
    const boton = form.querySelector('button[type="submit"], button:not([type])');
    if (boton) boton.disabled = true;
    const r = await api(ruta(form), { method: metodo, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(datos) });
    if (boton) boton.disabled = false;
    form.querySelector('.resultado').textContent = r.ok ? 'Guardado: ' + JSON.stringify(r.data) : mensajeError(r);
    if (r.ok) { cargarSelectores(); actualizarAvisos(); }
  });
}
async function cargarTabla(ruta, caja) {
  caja.replaceChildren(h('p', {}, 'Consultando…'));
  const r = await api(ruta);
  if (!r.ok) { caja.replaceChildren(h('p', { class: 'err' }, mensajeError(r))); return; }
  if (!Array.isArray(r.data)) { caja.replaceChildren(h('pre', {}, JSON.stringify(r.data, null, 2))); return; }
  if (!r.data.length) { caja.replaceChildren(h('p', {}, 'Sin registros.')); return; }
  const columnas = Object.keys(r.data[0]);
  caja.replaceChildren(h('p', {}, `${r.data.length} registros. Las listas muestran hasta 100; lotes hasta 500. La API permite paginar con limite y offset.`),
    h('table', {}, h('thead', {}, h('tr', {}, columnas.map(k => h('th', {}, k)))),
      h('tbody', {}, r.data.map(fila => h('tr', {}, columnas.map(k => h('td', {}, texto(fila[k]))))))));
}
function prepararReportes() {
  const hoy = new Date();
  const co = d => d.toLocaleDateString('en-CA', { timeZone: 'America/Bogota' });
  const anterior = new Date(hoy); anterior.setDate(anterior.getDate() - 29);
  $('#form-reporte').elements.desde.value = co(anterior);
  $('#form-reporte').elements.hasta.value = co(hoy);
}

const CAMPOS_PRODUCTO = [
  ['nombre', 'text', true], ['codigo_barras'], ['marca'], ['presentacion'],
  ['categoria_id', 'number', true], ['proveedor_id', 'number'], ['precio_venta', 'number', true, '0'],
  ['margen_porcentaje', 'number'], ['stock_minimo', 'number', true, '0'],
  ['dias_alerta_temprana', 'number', true, '120'], ['dias_alerta_liquidacion', 'number', true, '60'],
  ['es_granel', 'checkbox'], ['para_que_sirve', 'textarea'], ['modo_toma', 'textarea'],
  ['contraindicaciones', 'textarea'], ['ingredientes', 'textarea'],
  ['aviso_complemento', 'text', true, 'Es un complemento, no un medicamento. Ante algo serio, consulte a su médico.'], ['etiquetas', 'text', false, '']
];
function armarAdministracion() {
  const caja = $('#formularios-admin');
  const recursos = [
    ['productos', CAMPOS_PRODUCTO],
    ['categorias', [['nombre', 'text', true]]],
    ['proveedores', [['nombre', 'text', true], ['telefono'], ['acepta_devolucion', 'checkbox']]],
    ['etiquetas', [['nombre', 'text', true], ['tipo', 'text', true, 'necesidad'], ['activa', 'checkbox', false, true]]],
    ['usuarios', [['nombre', 'text', true], ['email', 'email', true], ['password', 'password'], ['rol', 'text', true, 'empleado'], ['estado', 'text', true, 'activo']]]
  ];
  caja.replaceChildren(...recursos.map(([recurso, campos]) => {
    const form = h('form', { method: 'post' }, h('h3', {}, recurso));
    const id = h('input', { type: 'number', min: '1', name: 'id', placeholder: 'Vacío para crear' });
    const salida = h('p', { role: 'status' });
    const lista = h('div', { class: 'scroll' });
    form.append(h('label', {}, 'ID para editar o eliminar ', id));
    campos.forEach(([nombre, tipo = 'text', requerido, inicial]) => {
      const campo = tipo === 'textarea' ? h('textarea', { name: nombre }) : h('input', { name: nombre, type: tipo, required: !!requerido });
      if (tipo === 'number') { campo.min = '0'; campo.step = ['precio_venta', 'margen_porcentaje'].includes(nombre) ? '0.01' : '1'; }
      if (tipo === 'checkbox') campo.checked = !!inicial;
      else if (inicial !== undefined) campo.value = inicial;
      if (tipo === 'password') { campo.minLength = 12; campo.maxLength = 72; campo.autocomplete = 'new-password'; }
      form.append(h('label', {}, nombre === 'etiquetas' ? 'IDs de etiquetas separados por coma' : nombre, ' ', campo));
    });
    const guardar = h('button', { type: 'submit' }, 'Guardar');
    form.append(guardar,
      h('button', { type: 'button', onclick: () => cargarTabla('/api/' + recurso, lista) }, 'Listar'),
      h('button', { type: 'button', onclick: async () => {
        if (!id.value) { salida.textContent = 'Indique el ID.'; return; }
        let r;
        if (recurso === 'productos' || recurso === 'categorias') r = await api('/api/' + recurso + '/' + id.value);
        else { r = await api('/api/' + recurso); if (r.ok) r.data = r.data.find(x => String(x.id) === id.value); }
        if (!r.ok || !r.data) { salida.textContent = r.ok ? 'Registro no encontrado.' : mensajeError(r); return; }
        let datos = r.data;
        if (recurso === 'productos') datos = { ...r.data.ficha, proveedor_id: r.data.ficha.proveedor?.id, etiquetas: r.data.etiquetas.map(x => x.id).join(',') };
        for (const [nombre, tipo] of campos) { if (nombre === 'password') { form.elements[nombre].value = ''; continue; } if (tipo === 'checkbox') form.elements[nombre].checked = !!datos[nombre]; else form.elements[nombre].value = datos[nombre] ?? ''; }
        salida.textContent = 'Datos cargados. Edite y guarde.';
      } }, 'Cargar para editar'));
    if (recurso !== 'usuarios') form.append(h('button', { type: 'button', onclick: async () => {
      if (!id.value) { salida.textContent = 'Indique el ID.'; return; }
      if (!window.confirm(`Eliminar ${recurso} #${id.value}${recurso === 'productos' ? ' (desactivar)' : ''}?`)) return;
      const r = await api('/api/' + recurso + '/' + id.value, { method: 'DELETE' });
      salida.textContent = r.ok ? 'Eliminado.' : mensajeError(r);
    } }, 'Eliminar'));
    form.append(salida, lista);
    form.addEventListener('submit', async e => {
      e.preventDefault();
      const datos = {};
      for (const [nombre, tipo] of campos) {
        const c = form.elements[nombre];
        datos[nombre] = tipo === 'checkbox' ? c.checked : tipo === 'number' ? (c.value === '' ? null : Number(c.value)) : c.value;
      }
      if (recurso === 'productos') {
        datos.etiquetas = form.elements.etiquetas.value.trim() ? form.elements.etiquetas.value.split(',').map(x => Number(x.trim())) : [];
        if (!datos.codigo_barras) datos.codigo_barras = null;
      }
      guardar.disabled = true;
      const r = await api('/api/' + recurso + (id.value ? '/' + id.value : ''), { method: id.value ? 'PUT' : 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(datos) });
      guardar.disabled = false;
      if (recurso === 'usuarios') form.elements.password.value = '';
      salida.textContent = r.ok ? 'Guardado: ' + JSON.stringify(r.data) : mensajeError(r);
      if (r.ok) actualizarAvisos();
    });
    return form;
  }));
}
