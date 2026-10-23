'use strict';

// ---------- Utilidades ----------

// h crea elementos del DOM. Todo el texto entra como textContent (nunca innerHTML),
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
const numero = (n) => new Intl.NumberFormat('es-CO').format(Number(n) || 0);

// "AAAA-MM-DD" -> "DD/MM/AAAA" sin pasar por Date (evita el corrimiento de zona horaria).
function fechaCO(iso) {
  if (!iso) return '—';
  const [a, m, d] = String(iso).slice(0, 10).split('-');
  return d && m && a ? `${d}/${m}/${a}` : iso;
}

const texto = (v) => (v === null || v === undefined || v === '' ? '—' : v);

// api hace la petición y devuelve {ok, status, data, ms}. Nunca lanza excepción.
async function api(ruta, opciones = {}) {
  const inicio = performance.now();
  try {
    const resp = await fetch(ruta, { headers: { Accept: 'application/json', ...(opciones.headers || {}) }, ...opciones });
    const crudo = await resp.text();
    let data = crudo;
    try { data = crudo ? JSON.parse(crudo) : null; } catch { /* respuesta no JSON */ }
    return { ok: resp.ok, status: resp.status, data, crudo, ms: Math.round(performance.now() - inicio) };
  } catch (err) {
    return { ok: false, status: 0, data: { error: 'Error de red: no se pudo contactar el servidor (' + err.message + ')' }, crudo: '', ms: Math.round(performance.now() - inicio) };
  }
}

const mensajeError = (r) => (r.data && r.data.error) ? r.data.error : `Error HTTP ${r.status}`;

function mostrarMsg(el, tipo, contenido) {
  el.className = 'msg msg-' + tipo;
  el.textContent = contenido;
  el.hidden = false;
}
const ocultarMsg = (el) => { el.hidden = true; };

function filaVacia(tbody, columnas, contenido) {
  tbody.replaceChildren(h('tr', {}, h('td', { class: 'vacio', colspan: columnas }, contenido)));
}

// ---------- Indicador de estado ----------

async function verificarSalud() {
  const caja = $('#estado');
  const txt = $('#estado-texto');
  const r = await api('/api/salud');
  if (r.ok && r.data && r.data.mysql) {
    caja.className = 'estado estado-ok';
    txt.textContent = 'Conectado a MySQL';
    caja.title = `MySQL ${r.data.mysql} · base ${r.data.base} · ${r.data.hora_servidor}`;
  } else {
    caja.className = 'estado estado-error';
    txt.textContent = 'Sin conexión: ' + mensajeError(r);
    caja.title = '';
  }
}

// ---------- Pestañas ----------

const cargadas = {};
const cargadores = {
  productos: () => buscarProductos(''),
  necesidades: cargarNecesidades,
  alertas: cargarAlertas,
  categorias: cargarCategorias,
  probador: armarProbador,
};

function activarPestana(nombre) {
  document.querySelectorAll('.pestanas button').forEach((b) => {
    b.setAttribute('aria-selected', String(b.dataset.tab === nombre));
  });
  document.querySelectorAll('.panel').forEach((p) => { p.hidden = p.id !== 'tab-' + nombre; });
  if (!cargadas[nombre] && cargadores[nombre]) {
    cargadas[nombre] = true;
    cargadores[nombre]();
  }
  if (location.hash !== '#' + nombre) history.replaceState(null, '', '#' + nombre);
}

// ---------- 1. Productos ----------

async function buscarProductos(q) {
  const tbody = $('#productos-cuerpo');
  const msg = $('#productos-msg');
  ocultarMsg(msg);
  filaVacia(tbody, 6, 'Cargando…');
  const r = await api('/api/productos?q=' + encodeURIComponent(q));
  if (!r.ok) {
    filaVacia(tbody, 6, '');
    mostrarMsg(msg, 'error', mensajeError(r));
    return;
  }
  if (r.data.length === 0) {
    filaVacia(tbody, 6, q ? `No hay productos que coincidan con «${q}».` : 'No hay productos activos.');
    return;
  }
  tbody.replaceChildren(...r.data.map((p) => h('tr', {
    class: p.stock_bajo ? 'bajo' : null,
    tabindex: 0,
    title: p.stock_bajo ? 'Stock en o por debajo del mínimo (' + p.stock_minimo + ')' : 'Ver ficha',
    onclick: () => abrirFicha(p.id),
    onkeydown: (e) => { if (e.key === 'Enter') abrirFicha(p.id); },
  },
  h('td', {}, h('strong', {}, p.nombre), p.es_granel ? h('span', { class: 'pill', style: 'margin-left:.4rem' }, 'granel') : null),
  h('td', {}, texto(p.marca)),
  h('td', {}, texto(p.presentacion)),
  h('td', {}, p.categoria),
  h('td', { class: 'num' }, pesos(p.precio_venta)),
  h('td', { class: 'num' }, numero(p.stock), ' ', h('small', {}, '/ mín ' + p.stock_minimo)),
  )));
}

async function abrirFicha(id) {
  const dlg = $('#ficha');
  const cuerpo = $('#ficha-cuerpo');
  $('#ficha-titulo').textContent = 'Cargando…';
  cuerpo.replaceChildren(h('p', { class: 'ayuda' }, 'Consultando ficha…'));
  if (!dlg.open) dlg.showModal();

  const r = await api('/api/productos/' + encodeURIComponent(id));
  if (!r.ok) {
    $('#ficha-titulo').textContent = 'No se pudo abrir la ficha';
    cuerpo.replaceChildren(h('div', { class: 'msg msg-error' }, mensajeError(r)));
    return;
  }
  const { ficha: f, etiquetas, lotes } = r.data;
  $('#ficha-titulo').textContent = f.nombre + (f.presentacion ? ' · ' + f.presentacion : '');

  const seccionTexto = (titulo, valor) => valor ? [h('h3', {}, titulo), h('p', {}, valor)] : null;

  cuerpo.replaceChildren(
    h('div', { class: 'aviso', role: 'note' }, h('span', { 'aria-hidden': 'true' }, '⚠️'), h('span', {}, f.aviso_complemento)),
    h('div', { class: 'datos' },
      dato('Marca', texto(f.marca)),
      dato('Categoría', f.categoria),
      dato('Precio', pesos(f.precio_venta)),
      dato('Stock vigente', numero(f.stock) + (f.stock_bajo ? ' (bajo)' : '')),
      dato('Stock mínimo', f.stock_minimo),
      dato('Código de barras', texto(f.codigo_barras)),
      dato('Proveedor', f.proveedor ? f.proveedor.nombre : '—'),
      dato('Devoluciones', f.proveedor ? (f.proveedor.acepta_devolucion ? 'Sí acepta' : 'No acepta') : '—'),
    ),
    h('h3', { style: 'font-size:.95rem;color:var(--verde);margin:.4rem 0' }, 'Etiquetas'),
    etiquetas.length
      ? h('div', { class: 'etiquetas' }, etiquetas.map((e) => h('span', { class: 'etq etq-' + e.tipo, title: e.tipo }, e.nombre)))
      : h('p', { class: 'ayuda' }, 'Sin etiquetas activas.'),
    h('div', { class: 'texto-ficha' },
      seccionTexto('¿Para qué sirve?', f.para_que_sirve),
      seccionTexto('Modo de uso', f.modo_toma),
      seccionTexto('Contraindicaciones', f.contraindicaciones),
      seccionTexto('Ingredientes', f.ingredientes),
    ),
    h('h3', { style: 'font-size:.95rem;color:var(--verde);margin:1rem 0 .4rem' }, 'Lotes vendibles (orden FEFO: primero el que vence primero)'),
    lotes.length
      ? h('div', { class: 'tabla-scroll' }, h('table', { class: 'tabla' },
        h('thead', {}, h('tr', {}, h('th', {}, '#'), h('th', {}, 'Lote'), h('th', {}, 'Vence'), h('th', { class: 'num' }, 'Días'), h('th', { class: 'num' }, 'Cantidad'))),
        h('tbody', {}, lotes.map((l, i) => h('tr', { class: i === 0 ? 'fila-devolver' : null, title: i === 0 ? 'Este lote se vende primero' : null },
          h('td', {}, i + 1),
          h('td', {}, texto(l.codigo_lote)),
          h('td', {}, fechaCO(l.fecha_vencimiento)),
          h('td', { class: 'num' }, l.dias_restantes),
          h('td', { class: 'num' }, numero(l.cantidad_actual)),
        ))),
      ))
      : h('div', { class: 'msg msg-aviso' }, 'No hay lotes vendibles (sin stock o vencidos).'),
  );
}

function dato(k, v) {
  return h('div', { class: 'dato' }, h('span', { class: 'k' }, k), h('span', { class: 'v' }, v));
}

// ---------- 2. ¿Qué necesita el cliente? ----------

async function cargarNecesidades() {
  const cont = $('#necesidades-botones');
  const msg = $('#necesidades-msg');
  cont.replaceChildren(h('span', { class: 'ayuda' }, 'Cargando necesidades…'));
  const r = await api('/api/etiquetas?tipo=necesidad');
  if (!r.ok) {
    cont.replaceChildren();
    mostrarMsg(msg, 'error', mensajeError(r));
    cargadas.necesidades = false;
    return;
  }
  ocultarMsg(msg);
  if (r.data.length === 0) {
    cont.replaceChildren(h('span', { class: 'ayuda' }, 'No hay etiquetas de necesidad activas.'));
    return;
  }
  cont.replaceChildren(...r.data.map((e) => h('button', {
    type: 'button', class: 'chip', 'aria-pressed': 'false',
    onclick: (ev) => {
      cont.querySelectorAll('.chip').forEach((c) => c.setAttribute('aria-pressed', 'false'));
      ev.currentTarget.setAttribute('aria-pressed', 'true');
      verRecomendados(e.id);
    },
  }, e.nombre, h('span', { class: 'n' }, '(' + e.cantidad_productos + ')'))));
}

async function verRecomendados(idEtiqueta) {
  const res = $('#necesidades-resultado');
  const msg = $('#necesidades-msg');
  ocultarMsg(msg);
  res.replaceChildren(h('p', { class: 'ayuda' }, 'Buscando productos…'));
  const r = await api('/api/etiquetas/' + encodeURIComponent(idEtiqueta) + '/productos');
  if (!r.ok) {
    res.replaceChildren();
    mostrarMsg(msg, 'error', mensajeError(r));
    return;
  }
  const { etiqueta, productos } = r.data;
  if (productos.length === 0) {
    res.replaceChildren(h('div', { class: 'msg msg-aviso' }, `No hay productos activos para «${etiqueta.nombre}».`));
    return;
  }
  res.replaceChildren(
    h('h2', {}, `Recomendados para «${etiqueta.nombre}»`),
    h('div', { class: 'tarjetas' }, productos.map((p) => {
      const estado = p.stock <= 0 ? ['agotado', 'Agotado'] : p.stock_bajo ? ['bajo', 'Quedan ' + numero(p.stock)] : ['', 'Disponibles: ' + numero(p.stock)];
      return h('button', { type: 'button', class: 'tarjeta', onclick: () => abrirFicha(p.id) },
        h('h3', {}, p.nombre),
        h('div', { class: 'meta' }, [p.marca, p.presentacion, p.categoria].filter(Boolean).join(' · ')),
        h('div', { class: 'fila-precio' },
          h('span', { class: 'precio' }, pesos(p.precio_venta)),
          h('span', { class: 'pill ' + estado[0] }, estado[1]),
        ),
      );
    })),
  );
}

// ---------- 3. Alertas ----------

async function cargarAlertas() {
  const [venc, stock] = await Promise.all([api('/api/alertas/vencimientos'), api('/api/alertas/stock-bajo')]);

  const tv = $('#venc-cuerpo');
  const mv = $('#venc-msg');
  if (!venc.ok) {
    filaVacia(tv, 8, '');
    mostrarMsg(mv, 'error', mensajeError(venc));
  } else if (venc.data.length === 0) {
    ocultarMsg(mv);
    filaVacia(tv, 8, 'Ningún lote con stock está dentro de la ventana de alerta. 🎉');
  } else {
    ocultarMsg(mv);
    tv.replaceChildren(...venc.data.map((a) => h('tr', { class: 'fila-' + a.nivel },
      h('td', {}, h('span', { class: 'nivel nivel-' + a.nivel }, a.nivel)),
      h('td', {}, h('strong', {}, a.producto), a.presentacion ? h('div', { class: 'ayuda', style: 'margin:0' }, a.presentacion) : null),
      h('td', {}, texto(a.codigo_lote)),
      h('td', {}, fechaCO(a.fecha_vencimiento)),
      h('td', { class: 'num' }, a.dias_restantes),
      h('td', { class: 'num' }, numero(a.cantidad_actual)),
      h('td', {}, texto(a.proveedor)),
      h('td', {}, a.acepta_devolucion === null ? '—' : (a.acepta_devolucion ? 'Sí' : 'No')),
    )));
  }

  const ts = $('#stock-cuerpo');
  const ms = $('#stock-msg');
  if (!stock.ok) {
    filaVacia(ts, 5, '');
    mostrarMsg(ms, 'error', mensajeError(stock));
  } else if (stock.data.length === 0) {
    ocultarMsg(ms);
    filaVacia(ts, 5, 'Todos los productos están por encima del stock mínimo.');
  } else {
    ocultarMsg(ms);
    ts.replaceChildren(...stock.data.map((p) => h('tr', { class: 'bajo' },
      h('td', {}, h('strong', {}, p.nombre)),
      h('td', {}, texto(p.marca)),
      h('td', {}, p.categoria),
      h('td', { class: 'num' }, numero(p.stock)),
      h('td', { class: 'num' }, numero(p.stock_minimo)),
    )));
  }
}

// ---------- 4. Categorías ----------

async function cargarCategorias(resaltarId) {
  const ul = $('#cat-lista');
  const msg = $('#cat-msg');
  const r = await api('/api/categorias');
  if (!r.ok) {
    ul.replaceChildren();
    mostrarMsg(msg, 'error', mensajeError(r));
    return;
  }
  ocultarMsg(msg);
  $('#cat-conteo').textContent = '(' + r.data.length + ')';
  ul.replaceChildren(...r.data.map((c) => h('li', { class: c.id === resaltarId ? 'nueva' : null },
    h('span', {}, c.nombre), h('span', { class: 'id' }, 'id ' + c.id))));
}

async function crearCategoria(ev) {
  ev.preventDefault();
  const input = $('#cat-nombre');
  const res = $('#cat-resultado');
  const btn = ev.submitter || $('#form-categoria .btn');
  btn.disabled = true;
  const r = await api('/api/categorias', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ nombre: input.value }),
  });
  btn.disabled = false;
  if (r.status === 201) {
    mostrarMsg(res, 'ok', `201 Creada: «${r.data.nombre}» (id ${r.data.id}).`);
    input.value = '';
    cargarCategorias(r.data.id);
  } else if (r.status === 400) {
    mostrarMsg(res, 'error', '400 Datos inválidos: ' + mensajeError(r));
  } else if (r.status === 409) {
    mostrarMsg(res, 'aviso', '409 Duplicada: ' + mensajeError(r));
  } else {
    mostrarMsg(res, 'error', (r.status ? r.status + ' ' : '') + mensajeError(r));
  }
}

// ---------- 5. Probador de API ----------

const ENDPOINTS = [
  { metodo: 'GET', ruta: '/api/salud', desc: 'Estado del API y de la conexión con MySQL' },
  { metodo: 'GET', ruta: '/api/categorias', desc: 'Lista de categorías' },
  { metodo: 'POST', ruta: '/api/categorias', desc: 'Crear categoría (201 creada · 400 inválida · 409 duplicada)', cuerpo: '{\n  "nombre": "Categoría de prueba"\n}' },
  { metodo: 'GET', ruta: '/api/productos?q=', desc: 'Buscar productos activos por nombre, marca o código de barras' },
  { metodo: 'GET', ruta: '/api/productos/1', desc: 'Ficha del producto + etiquetas + lotes FEFO (404 si no existe)' },
  { metodo: 'GET', ruta: '/api/etiquetas?tipo=necesidad', desc: 'Etiquetas activas por tipo (necesidad, contraindicacion, ingrediente, otro)' },
  { metodo: 'GET', ruta: '/api/etiquetas/1/productos', desc: 'Productos recomendados para una etiqueta, con stock' },
  { metodo: 'GET', ruta: '/api/alertas/stock-bajo', desc: 'Productos con stock vigente ≤ stock mínimo' },
  { metodo: 'GET', ruta: '/api/alertas/vencimientos', desc: 'Lotes dentro de la ventana de alerta con su nivel' },
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

async function copiarTexto(textoACopiar) {
  try {
    await navigator.clipboard.writeText(textoACopiar);
    return true;
  } catch {
    // Respaldo para HTTP sin contexto seguro.
    const ta = h('textarea', { style: 'position:fixed;opacity:0' });
    ta.value = textoACopiar;
    document.body.append(ta);
    ta.select();
    let ok = false;
    try { ok = document.execCommand('copy'); } catch { ok = false; }
    ta.remove();
    return ok;
  }
}

function armarProbador() {
  const lista = $('#probador-lista');
  lista.replaceChildren(...ENDPOINTS.map((ep) => {
    const inputRuta = h('input', { type: 'text', value: ep.ruta, 'aria-label': 'Ruta', spellcheck: 'false' });
    const area = ep.cuerpo !== undefined ? h('textarea', { rows: 4, 'aria-label': 'Cuerpo JSON', spellcheck: 'false' }) : null;
    if (area) area.value = ep.cuerpo;
    const salida = h('div', { class: 'respuesta', hidden: true });
    const curlInfo = h('div', { class: 'curl ayuda', hidden: true });

    const enviar = h('button', { type: 'button', class: 'btn' }, 'Enviar');
    enviar.addEventListener('click', async () => {
      let ruta = inputRuta.value.trim();
      if (!ruta.startsWith('/')) ruta = '/' + ruta;
      const opciones = { method: ep.metodo };
      if (area) {
        opciones.headers = { 'Content-Type': 'application/json' };
        opciones.body = area.value;
      }
      enviar.disabled = true;
      const r = await api(ruta, opciones);
      enviar.disabled = false;
      const cuerpoTxt = (r.data !== null && typeof r.data === 'object') ? JSON.stringify(r.data, null, 2) : (r.crudo || (r.data && r.data.error) || '(sin cuerpo)');
      salida.replaceChildren(
        h('div', { class: 'info' },
          h('span', { class: 'codigo codigo-' + String(r.status).charAt(0) }, r.status === 0 ? 'Sin respuesta' : 'HTTP ' + r.status),
          h('span', {}, r.ms + ' ms'),
          Array.isArray(r.data) ? h('span', {}, r.data.length + ' elementos') : null,
        ),
        h('pre', { class: 'json' }, cuerpoTxt),
      );
      salida.hidden = false;
    });

    const copiar = h('button', { type: 'button', class: 'btn btn-sec' }, 'Copiar como curl');
    copiar.addEventListener('click', async () => {
      const cmd = comandoCurl(ep.metodo, inputRuta.value.trim(), area ? area.value : undefined);
      const ok = await copiarTexto(cmd);
      curlInfo.replaceChildren(ok ? '✔ Copiado: ' : 'Copia manualmente: ', h('code', {}, cmd));
      curlInfo.hidden = false;
    });

    return h('div', { class: 'endpoint' },
      h('div', { class: 'titulo' }, ep.desc),
      h('div', { class: 'linea' }, h('span', { class: 'metodo metodo-' + ep.metodo }, ep.metodo), inputRuta, enviar, copiar),
      area,
      curlInfo,
      salida,
    );
  }));
}

// ---------- Inicio ----------

document.addEventListener('DOMContentLoaded', () => {
  document.querySelectorAll('.pestanas button').forEach((b) => b.addEventListener('click', () => activarPestana(b.dataset.tab)));

  let temporizador;
  $('#form-buscar').addEventListener('submit', (e) => { e.preventDefault(); buscarProductos($('#q').value.trim()); });
  $('#q').addEventListener('input', () => {
    clearTimeout(temporizador);
    temporizador = setTimeout(() => buscarProductos($('#q').value.trim()), 300);
  });

  $('#form-categoria').addEventListener('submit', crearCategoria);

  const dlg = $('#ficha');
  $('#ficha-cerrar').addEventListener('click', () => dlg.close());
  dlg.addEventListener('click', (e) => { if (e.target === dlg) dlg.close(); });

  verificarSalud();
  setInterval(verificarSalud, 30000);

  const inicial = location.hash.slice(1);
  activarPestana(cargadores[inicial] ? inicial : 'productos');
});
