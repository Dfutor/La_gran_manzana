'use strict';

// ---------- Utilidades ----------

// h crea elementos del DOM. El texto entra siempre como textContent (nunca innerHTML),
// así los datos de la base no pueden inyectar HTML.
function h(tag, attrs, ...hijos) {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v === null || v === undefined || v === false) continue;
    if (k === 'class') el.className = v;
    else if (k.startsWith('on')) el.addEventListener(k.slice(2), v);
    else el.setAttribute(k, v === true ? '' : v);
  }
  for (const hijo of hijos.flat()) {
    if (hijo === null || hijo === undefined || hijo === false) continue;
    el.append(hijo instanceof Node ? hijo : String(hijo));
  }
  return el;
}

const $ = (sel) => document.querySelector(sel);
const fmtPesos = new Intl.NumberFormat('es-CO', { style: 'currency', currency: 'COP', maximumFractionDigits: 0, minimumFractionDigits: 0 });
const pesos = (n) => fmtPesos.format(Number(n) || 0);
const texto = (v) => (v === null || v === undefined || v === '' ? '—' : v);

// "AAAA-MM-DD" -> "DD/MM/AAAA" sin pasar por Date (evita el corrimiento de zona horaria).
function fechaCO(iso) {
  if (!iso) return '—';
  const [a, m, d] = String(iso).slice(0, 10).split('-');
  return d && m && a ? `${d}/${m}/${a}` : iso;
}

// api hace la petición y devuelve {ok, status, data, crudo, ms}. Nunca lanza excepción.
async function api(ruta, opciones = {}) {
  const inicio = performance.now();
  try {
    const resp = await fetch(ruta, { ...opciones, headers: { Accept: 'application/json', ...(opciones.headers || {}) } });
    const crudo = await resp.text();
    let data = null;
    try { data = crudo ? JSON.parse(crudo) : null; } catch { data = null; }
    return { ok: resp.ok, status: resp.status, data, crudo, ms: Math.round(performance.now() - inicio) };
  } catch (err) {
    return { ok: false, status: 0, data: { error: 'Error de red: ' + err.message }, crudo: '', ms: Math.round(performance.now() - inicio) };
  }
}

const mensajeError = (r) => (r.data && r.data.error) ? r.data.error : `Error HTTP ${r.status}`;

function msg(el, clase, contenido) {
  el.className = 'msg ' + clase;
  el.textContent = contenido || '';
}

function filaUnica(tbody, columnas, contenido) {
  tbody.replaceChildren(h('tr', {}, h('td', { colspan: columnas }, contenido)));
}

// ---------- Estado ----------

async function verificarSalud() {
  const el = $('#estado');
  const r = await api('/api/salud');
  if (r.ok && r.data && r.data.mysql) {
    el.className = 'ok';
    el.textContent = `Conectado a MySQL ${r.data.mysql} (base ${r.data.base})`;
  } else {
    el.className = 'err';
    el.textContent = 'Sin conexión: ' + mensajeError(r);
  }
}

// ---------- Pestañas ----------

const cargadas = {};
const cargadores = {
  productos: () => buscarProductos(''),
  necesidades: cargarNecesidades,
  alertas: cargarAlertas,
  categorias: () => cargarCategorias(),
  probador: armarProbador,
};

function activarPestana(nombre) {
  document.querySelectorAll('#pestanas button').forEach((b) => b.setAttribute('aria-selected', String(b.dataset.tab === nombre)));
  document.querySelectorAll('.panel').forEach((p) => { p.hidden = p.id !== 'tab-' + nombre; });
  if (!cargadas[nombre]) {
    cargadas[nombre] = true;
    cargadores[nombre]();
  }
  history.replaceState(null, '', '#' + nombre);
}

// ---------- 1. Productos ----------

async function buscarProductos(q) {
  const tbody = $('#productos-cuerpo');
  const m = $('#productos-msg');
  msg(m, '', '');
  filaUnica(tbody, 6, 'Cargando…');
  const r = await api('/api/productos?q=' + encodeURIComponent(q));
  if (!r.ok) { filaUnica(tbody, 6, ''); msg(m, 'err', mensajeError(r)); return; }
  if (r.data.length === 0) { filaUnica(tbody, 6, 'Sin resultados.'); return; }
  tbody.replaceChildren(...r.data.map((p) => h('tr', {
    class: 'clic' + (p.stock_bajo ? ' bajo' : ''),
    tabindex: 0,
    onclick: () => verFicha(p.id),
    onkeydown: (e) => { if (e.key === 'Enter') verFicha(p.id); },
  },
  h('td', {}, p.nombre, p.es_granel ? ' (granel)' : ''),
  h('td', {}, texto(p.marca)),
  h('td', {}, texto(p.presentacion)),
  h('td', {}, p.categoria),
  h('td', {}, pesos(p.precio_venta)),
  h('td', {}, `${p.stock} (mín ${p.stock_minimo})`),
  )));
}

async function verFicha(id) {
  const caja = $('#ficha');
  caja.replaceChildren(h('p', {}, 'Cargando ficha…'));
  const r = await api('/api/productos/' + encodeURIComponent(id));
  if (!r.ok) { caja.replaceChildren(h('p', { class: 'err' }, mensajeError(r))); return; }
  const { ficha: f, etiquetas, lotes } = r.data;
  const parrafo = (titulo, valor) => (valor ? h('p', {}, h('b', {}, titulo + ': '), valor) : null);
  caja.replaceChildren(
    h('h2', {}, f.nombre + (f.presentacion ? ' · ' + f.presentacion : '')),
    h('p', { class: 'aviso' }, '⚠ ' + f.aviso_complemento),
    h('p', {}, `Marca: ${texto(f.marca)} · Categoría: ${f.categoria} · Precio: ${pesos(f.precio_venta)} · Stock vigente: ${f.stock}` +
      (f.stock_vencido ? ` · Vencido en estante: ${f.stock_vencido}` : '') +
      ` · Proveedor: ${f.proveedor ? f.proveedor.nombre + (f.proveedor.acepta_devolucion ? ' (acepta devolución)' : ' (no acepta devolución)') : '—'}`),
    h('p', {}, h('b', {}, 'Etiquetas: '), etiquetas.length ? etiquetas.map((e) => `${e.nombre} [${e.tipo}]`).join(', ') : 'ninguna'),
    parrafo('Para qué sirve', f.para_que_sirve),
    parrafo('Modo de uso', f.modo_toma),
    parrafo('Contraindicaciones', f.contraindicaciones),
    parrafo('Ingredientes', f.ingredientes),
    h('p', {}, h('b', {}, 'Lotes en orden FEFO (se vende primero el de arriba):')),
    lotes.length
      ? h('table', {},
        h('tr', {}, h('th', {}, 'Lote'), h('th', {}, 'Vence'), h('th', {}, 'Días'), h('th', {}, 'Cantidad')),
        lotes.map((l) => h('tr', {}, h('td', {}, texto(l.codigo_lote)), h('td', {}, fechaCO(l.fecha_vencimiento)), h('td', {}, l.dias_restantes), h('td', {}, l.cantidad_actual))))
      : h('p', { class: 'err' }, 'No hay lotes vendibles.'),
  );
  caja.scrollIntoView({ behavior: 'smooth' });
}

// ---------- 2. ¿Qué necesita el cliente? ----------

let necesidadActual = null;

async function cargarNecesidades() {
  const m = $('#necesidades-msg');
  const [nec, contra] = await Promise.all([api('/api/etiquetas?tipo=necesidad'), api('/api/etiquetas?tipo=contraindicacion')]);
  if (!nec.ok || !contra.ok) { msg(m, 'err', mensajeError(nec.ok ? contra : nec)); cargadas.necesidades = false; return; }
  $('#necesidades-botones').replaceChildren(...nec.data.map((e) => h('button', {
    type: 'button',
    onclick: (ev) => {
      document.querySelectorAll('#necesidades-botones button').forEach((b) => b.setAttribute('aria-pressed', 'false'));
      ev.currentTarget.setAttribute('aria-pressed', 'true');
      necesidadActual = e.id;
      verRecomendados();
    },
  }, `${e.nombre} (${e.cantidad_productos})`)));
  $('#contra-opciones').replaceChildren(...contra.data.map((e) => h('label', { style: 'display:block' },
    h('input', { type: 'checkbox', value: e.id, onchange: verRecomendados }), ' ' + e.nombre)));
  $('#solo-disponibles').addEventListener('change', verRecomendados);
}

async function verRecomendados() {
  if (!necesidadActual) return;
  const res = $('#necesidades-resultado');
  const m = $('#necesidades-msg');
  msg(m, '', '');
  const evitar = [...document.querySelectorAll('#contra-opciones input:checked')].map((c) => c.value).join(',');
  const params = new URLSearchParams({ necesidad: necesidadActual });
  if (evitar) params.set('evitar', evitar);
  if ($('#solo-disponibles').checked) params.set('disponibles', '1');
  res.replaceChildren(h('p', {}, 'Buscando…'));
  const r = await api('/api/recomendaciones?' + params);
  if (!r.ok) { res.replaceChildren(); msg(m, 'err', mensajeError(r)); return; }
  const { necesidad, evitando, productos, aviso } = r.data;
  res.replaceChildren(
    h('h2', {}, `Para «${necesidad.nombre}»` + (evitando.length ? ` evitando: ${evitando.map((e) => e.nombre).join(', ')}` : '')),
    productos.length
      ? h('ul', {}, productos.map((p) => h('li', { class: p.stock <= 0 ? 'err' : null },
        h('a', { href: '#necesidades', onclick: (e) => { e.preventDefault(); activarPestana('productos'); verFicha(p.id); } }, p.nombre),
        ` — ${pesos(p.precio_venta)} — ${p.stock > 0 ? 'stock ' + p.stock : 'agotado'}`)))
      : h('p', {}, 'No hay productos que cumplan los filtros.'),
    h('p', { class: 'aviso' }, aviso),
  );
}

// ---------- 3. Alertas ----------

async function cargarAlertas() {
  const [venc, stock] = await Promise.all([api('/api/alertas/vencimientos'), api('/api/alertas/stock-bajo')]);

  const tv = $('#venc-cuerpo');
  if (!venc.ok) { filaUnica(tv, 8, ''); msg($('#venc-msg'), 'err', mensajeError(venc)); }
  else if (venc.data.length === 0) filaUnica(tv, 8, 'Ningún lote en la ventana de alerta.');
  else {
    tv.replaceChildren(...venc.data.map((a) => h('tr', { class: a.nivel },
      h('td', {}, a.nivel.toUpperCase()),
      h('td', {}, a.producto),
      h('td', {}, texto(a.codigo_lote)),
      h('td', {}, fechaCO(a.fecha_vencimiento)),
      h('td', {}, a.dias_restantes),
      h('td', {}, a.cantidad_actual),
      h('td', {}, pesos(a.valor_en_riesgo)),
      h('td', {}, a.accion_sugerida),
    )));
  }

  const ts = $('#stock-cuerpo');
  if (!stock.ok) { filaUnica(ts, 5, ''); msg($('#stock-msg'), 'err', mensajeError(stock)); }
  else if (stock.data.length === 0) filaUnica(ts, 5, 'Todo por encima del mínimo.');
  else {
    ts.replaceChildren(...stock.data.map((p) => h('tr', { class: 'bajo' },
      h('td', {}, p.nombre), h('td', {}, p.categoria), h('td', {}, p.stock), h('td', {}, p.stock_minimo), h('td', {}, p.faltante_para_minimo))));
  }
}

// ---------- 4. Categorías ----------

async function cargarCategorias(resaltarId) {
  const ul = $('#cat-lista');
  const r = await api('/api/categorias');
  if (!r.ok) { ul.replaceChildren(h('li', { class: 'err' }, mensajeError(r))); return; }
  ul.replaceChildren(...r.data.map((c) => h('li', { class: c.id === resaltarId ? 'ok' : null },
    `${c.nombre} (id ${c.id}, ${c.cantidad_productos} productos) `,
    c.cantidad_productos === 0 ? h('button', { type: 'button', onclick: () => eliminarCategoria(c.id, c.nombre) }, 'Eliminar') : null)));
}

async function crearCategoria(ev) {
  ev.preventDefault();
  const input = $('#cat-nombre');
  const res = $('#cat-resultado');
  const r = await api('/api/categorias', {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ nombre: input.value }),
  });
  if (r.status === 201) {
    msg(res, 'ok', `201 Creada: «${r.data.nombre}» (id ${r.data.id}).`);
    input.value = '';
    cargarCategorias(r.data.id);
  } else {
    msg(res, 'err', `${r.status || 'Sin respuesta'}: ${mensajeError(r)}`);
  }
}

async function eliminarCategoria(id, nombre) {
  const res = $('#cat-resultado');
  const r = await api('/api/categorias/' + id, { method: 'DELETE' });
  if (r.status === 204) msg(res, 'ok', `204 Eliminada: «${nombre}».`);
  else msg(res, 'err', `${r.status || 'Sin respuesta'}: ${mensajeError(r)}`);
  cargarCategorias();
}

// ---------- 5. Probador de API ----------

const ENDPOINTS = [
  { metodo: 'GET', ruta: '/api/salud' },
  { metodo: 'GET', ruta: '/api/productos?q=' },
  { metodo: 'GET', ruta: '/api/productos/1' },
  { metodo: 'GET', ruta: '/api/productos/1/plan-fefo?cantidad=40' },
  { metodo: 'GET', ruta: '/api/categorias' },
  { metodo: 'GET', ruta: '/api/categorias/1' },
  { metodo: 'POST', ruta: '/api/categorias', cuerpo: '{\n  "nombre": "Categoría de prueba"\n}' },
  { metodo: 'PUT', ruta: '/api/categorias/1', cuerpo: '{\n  "nombre": "Categoría actualizada"\n}' },
  { metodo: 'DELETE', ruta: '/api/categorias/ID' },
  { metodo: 'GET', ruta: '/api/proveedores' },
  { metodo: 'POST', ruta: '/api/proveedores', cuerpo: '{\n  "nombre": "Proveedor de prueba",\n  "telefono": "3000000000",\n  "acepta_devolucion": true\n}' },
  { metodo: 'PUT', ruta: '/api/proveedores/1', cuerpo: '{\n  "nombre": "Proveedor actualizado",\n  "telefono": "3000000000",\n  "acepta_devolucion": false\n}' },
  { metodo: 'DELETE', ruta: '/api/proveedores/ID' },
  { metodo: 'POST', ruta: '/api/productos', cuerpo: '{\n  "nombre": "Producto de prueba",\n  "categoria_id": 1,\n  "precio_venta": 10000,\n  "stock_minimo": 0\n}' },
  { metodo: 'PUT', ruta: '/api/productos/1', cuerpo: '{\n  "nombre": "Producto actualizado",\n  "categoria_id": 1,\n  "precio_venta": 12000,\n  "stock_minimo": 1\n}' },
  { metodo: 'DELETE', ruta: '/api/productos/ID' },
  { metodo: 'GET', ruta: '/api/etiquetas?tipo=necesidad' },
  { metodo: 'POST', ruta: '/api/etiquetas', cuerpo: '{\n  "nombre": "Etiqueta de prueba",\n  "tipo": "otro"\n}' },
  { metodo: 'PUT', ruta: '/api/etiquetas/1', cuerpo: '{\n  "nombre": "Etiqueta actualizada",\n  "tipo": "otro"\n}' },
  { metodo: 'DELETE', ruta: '/api/etiquetas/ID' },
  { metodo: 'GET', ruta: '/api/etiquetas/3/productos' },
  { metodo: 'GET', ruta: '/api/recomendaciones?necesidad=3&evitar=13&disponibles=1' },
  { metodo: 'GET', ruta: '/api/alertas/stock-bajo' },
  { metodo: 'GET', ruta: '/api/alertas/vencimientos' },
];

const comillaShell = (s) => "'" + String(s).replace(/'/g, "'\\''") + "'";

function comandoCurl(metodo, ruta, cuerpo) {
  const url = window.location.origin + (ruta.startsWith('/') ? ruta : '/' + ruta);
  let cmd = 'curl -i';
  if (metodo !== 'GET') cmd += ' -X ' + metodo;
  cmd += ' ' + comillaShell(url);
  if (cuerpo !== undefined) cmd += " -H 'Content-Type: application/json' -d " + comillaShell(cuerpo);
  return cmd;
}

async function copiarTexto(t) {
  try { await navigator.clipboard.writeText(t); return true; } catch { /* HTTP sin contexto seguro */ }
  const ta = h('textarea', {});
  ta.value = t;
  document.body.append(ta);
  ta.select();
  let ok = false;
  try { ok = document.execCommand('copy'); } catch { ok = false; }
  ta.remove();
  return ok;
}

function armarProbador() {
  $('#probador-lista').replaceChildren(...ENDPOINTS.map((ep) => {
    const ruta = h('input', { type: 'text', value: ep.ruta, 'aria-label': 'Ruta', spellcheck: 'false' });
    const area = ep.cuerpo !== undefined ? h('textarea', { rows: 3, 'aria-label': 'Cuerpo JSON' }) : null;
    if (area) area.value = ep.cuerpo;
    const info = h('div', {});
    const salida = h('pre', { hidden: true });

    const enviar = async () => {
      let r0 = ruta.value.trim();
      if (!r0.startsWith('/')) r0 = '/' + r0;
      const opciones = { method: ep.metodo };
      if (area) { opciones.headers = { 'Content-Type': 'application/json' }; opciones.body = area.value; }
      const r = await api(r0, opciones);
      info.replaceChildren(h('b', { class: r.ok ? 'ok' : 'err' }, r.status ? 'HTTP ' + r.status : 'Sin respuesta'), ` · ${r.ms} ms`);
      salida.textContent = r.data !== null ? JSON.stringify(r.data, null, 2) : (r.crudo || '(sin cuerpo)');
      salida.hidden = false;
    };
    const copiar = async () => {
      const cmd = comandoCurl(ep.metodo, ruta.value.trim(), area ? area.value : undefined);
      const ok = await copiarTexto(cmd);
      info.replaceChildren(ok ? 'Copiado: ' : 'Copia manualmente: ', h('code', {}, cmd));
    };

    return h('div', { class: 'endpoint' },
      h('b', {}, ep.metodo + ' '), ruta, ' ',
      h('button', { type: 'button', onclick: enviar }, 'Enviar'), ' ',
      h('button', { type: 'button', onclick: copiar }, 'Copiar como curl'),
      area, info, salida);
  }));
}

// ---------- Inicio ----------

document.addEventListener('DOMContentLoaded', () => {
  document.querySelectorAll('#pestanas button').forEach((b) => b.addEventListener('click', () => activarPestana(b.dataset.tab)));
  $('#form-buscar').addEventListener('submit', (e) => { e.preventDefault(); buscarProductos($('#q').value.trim()); });
  $('#form-categoria').addEventListener('submit', crearCategoria);
  verificarSalud();
  setInterval(verificarSalud, 30000);
  const inicial = location.hash.slice(1);
  activarPestana(cargadores[inicial] ? inicial : 'productos');
});
