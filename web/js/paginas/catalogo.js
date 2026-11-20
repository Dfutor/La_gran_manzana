// Catálogo: administración de productos, categorías, proveedores y etiquetas.
import { h, pesos, numero, texto, normalizar } from '../nucleo/dom.js';
import { get, post, put, del } from '../nucleo/api.js';
import { puede } from '../nucleo/sesion.js';
import { tabla, insignia, vacio, esqueletoTabla, errorCarga, boton, botonIcono, campo, notificar, pestanas, modalFormulario, confirmar, selector, paginador, interruptorConEtiqueta, esqueleto, aviso } from '../nucleo/ui.js';
import { icono } from '../nucleo/iconos.js';

const TIPOS_ETIQUETA = [['necesidad', 'Necesidad'], ['ingrediente', 'Ingrediente'], ['contraindicacion', 'Contraindicación'], ['otro', 'Otro']];

// Interruptor que viaja dentro de un formulario: guarda su valor en un input oculto.
function interruptorCampo(nombre, textoEtq, activo) {
  const oculto = h('input', { type: 'hidden', name: nombre, value: activo ? '1' : '0' });
  const { elemento } = interruptorConEtiqueta(textoEtq, { activo, alCambiar: (v) => { oculto.value = v ? '1' : '0'; } });
  return h('div', { class: 'campo', style: { justifyContent: 'flex-end' } }, oculto, elemento);
}
const nulo = (v) => (v === undefined || String(v).trim() === '' ? null : String(v).trim());

// Lista genérica con búsqueda, botón «Nuevo» y acciones por fila.
function recursoCrud({ recurso, singular, columnas, cargar, formulario, guardar, eliminar, textoEliminar }) {
  const buscar = h('input', { type: 'search', placeholder: `Buscar ${singular}…`, 'aria-label': `Buscar ${singular}` });
  const caja = h('div', {}, esqueletoTabla());
  const pag = paginador(12, () => pintar());
  let datos = [];

  function pintar() {
    const q = normalizar(buscar.value);
    const lista = q ? datos.filter((d) => normalizar(d.nombre).includes(q)) : datos;
    const cols = [...columnas];
    if (puede(`${recurso}.editar`) || puede(`${recurso}.eliminar`)) {
      cols.push({
        titulo: '', clase: 'derecha', valor: (d) => h('div', { class: 'acciones' },
          puede(`${recurso}.editar`) ? botonIcono('pencil', `Editar ${d.nombre}`, { onclick: () => abrir(d) }) : null,
          puede(`${recurso}.eliminar`) ? botonIcono('trash-2', `${textoEliminar} ${d.nombre}`, { onclick: () => quitar(d) }) : null),
      });
    }
    caja.replaceChildren(lista.length ? tabla(cols, pag.pagina(lista), { etiqueta: recurso }) : vacio({ icono: 'search', titulo: `No hay ${recurso}`, texto: q ? 'Pruebe con otra búsqueda.' : '' }), pag.pie);
  }
  async function recargar() {
    caja.replaceChildren(esqueletoTabla());
    const r = await cargar();
    if (!r.ok) { caja.replaceChildren(errorCarga(r, recargar)); return; }
    datos = r.data;
    pintar();
  }
  async function abrir(d) {
    const campos = await formulario(d);
    modalFormulario({
      titulo: d ? `Editar ${singular}` : `Nuevo ${singular}`, campos, ancho: recurso === 'productos',
      enviar: async (f) => {
        const r = await guardar(f, d);
        if (r.ok) { notificar(`${singular[0].toUpperCase() + singular.slice(1)} guardado.`, 'exito'); recargar(); }
        return r;
      },
    });
  }
  async function quitar(d) {
    const ok = await confirmar({ titulo: `${textoEliminar} ${singular}`, mensaje: `¿${textoEliminar} «${d.nombre}»?${recurso === 'productos' ? ' El historial de ventas se conserva.' : ''}`, confirmar: textoEliminar, peligro: true });
    if (!ok) return;
    const r = await eliminar(d);
    notificar(r.ok ? `«${d.nombre}» eliminado.` : r.error, r.ok ? 'exito' : 'error');
    if (r.ok) recargar();
  }
  buscar.addEventListener('input', () => { pag.reiniciar(); pintar(); });
  recargar();
  return h('div', { class: 'pila' },
    h('div', { class: 'herramientas' },
      h('div', { class: 'campo busqueda' }, h('div', { class: 'entrada-icono' }, icono('search', { tam: 18 }), buscar)),
      puede(`${recurso}.crear`) ? boton(`Nuevo ${singular}`, { tipo: 'primario', icono: 'plus', onclick: () => abrir(null) }) : null),
    h('div', { class: 'tarjeta tarjeta-sin-relleno' }, caja));
}

function categorias() {
  return recursoCrud({
    recurso: 'categorias', singular: 'categoría', textoEliminar: 'Eliminar',
    cargar: () => get('/api/categorias'),
    columnas: [
      { titulo: 'Categoría', valor: (c) => h('span', { class: 'negrita' }, c.nombre) },
      { titulo: 'Productos', clase: 'derecha num', valor: (c) => numero(c.cantidad_productos) },
    ],
    formulario: (c) => campo('Nombre', h('input', { name: 'nombre', required: true, maxlength: '80', value: c?.nombre || '' })),
    guardar: (f, c) => (c ? put : post)('/api/categorias' + (c ? '/' + c.id : ''), { nombre: f.elements.nombre.value }),
    eliminar: (c) => del('/api/categorias/' + c.id),
  });
}

function proveedores() {
  return recursoCrud({
    recurso: 'proveedores', singular: 'proveedor', textoEliminar: 'Eliminar',
    cargar: () => get('/api/proveedores'),
    columnas: [
      { titulo: 'Proveedor', valor: (p) => h('span', { class: 'negrita' }, p.nombre) },
      { titulo: 'Teléfono', valor: (p) => texto(p.telefono) },
      { titulo: 'Devoluciones', valor: (p) => (p.acepta_devolucion ? insignia('Acepta', 'exito') : insignia('No acepta')) },
      { titulo: 'Productos', clase: 'derecha num', valor: (p) => numero(p.cantidad_productos) },
    ],
    formulario: (p) => h('div', { class: 'rejilla-form' },
      campo('Nombre', h('input', { name: 'nombre', required: true, maxlength: '120', value: p?.nombre || '' })),
      campo('Teléfono', h('input', { name: 'telefono', type: 'tel', maxlength: '30', value: p?.telefono || '' })),
      interruptorCampo('acepta_devolucion', 'Acepta devoluciones', !!p?.acepta_devolucion)),
    guardar: (f, p) => (p ? put : post)('/api/proveedores' + (p ? '/' + p.id : ''), {
      nombre: f.elements.nombre.value, telefono: nulo(f.elements.telefono.value), acepta_devolucion: f.elements.acepta_devolucion.value === '1',
    }),
    eliminar: (p) => del('/api/proveedores/' + p.id),
  });
}

function etiquetas() {
  const tipo = Object.fromEntries(TIPOS_ETIQUETA);
  return recursoCrud({
    recurso: 'etiquetas', singular: 'etiqueta', textoEliminar: 'Desactivar',
    cargar: () => get('/api/etiquetas'),
    columnas: [
      { titulo: 'Etiqueta', valor: (e) => h('span', { class: 'negrita' }, e.nombre) },
      { titulo: 'Tipo', valor: (e) => insignia(tipo[e.tipo] || e.tipo, e.tipo === 'contraindicacion' ? 'error' : e.tipo === 'necesidad' ? 'exito' : 'info') },
      { titulo: 'Productos', clase: 'derecha num', valor: (e) => numero(e.cantidad_productos) },
    ],
    formulario: (e) => h('div', { class: 'rejilla-form' },
      campo('Nombre', h('input', { name: 'nombre', required: true, maxlength: '80', value: e?.nombre || '' })),
      campo('Tipo', selector(TIPOS_ETIQUETA.map(([v, t]) => ({ valor: v, texto: t, seleccionado: v === (e?.tipo || 'necesidad') })), { name: 'tipo' }))),
    guardar: (f, e) => (e ? put : post)('/api/etiquetas' + (e ? '/' + e.id : ''), { nombre: f.elements.nombre.value, tipo: f.elements.tipo.value, ...(e ? { activa: true } : {}) }),
    eliminar: (e) => del('/api/etiquetas/' + e.id),
  });
}

function productosCrud() {
  return recursoCrud({
    recurso: 'productos', singular: 'producto', textoEliminar: 'Desactivar',
    cargar: () => get('/api/productos'),
    columnas: [
      { titulo: 'Producto', valor: (p) => h('span', {}, h('span', { class: 'negrita' }, p.nombre), h('span', { class: 'sub' }, [p.marca, p.presentacion].filter(Boolean).join(' · ') || '—')) },
      { titulo: 'Categoría', valor: (p) => p.categoria },
      { titulo: 'Precio', clase: 'derecha num', valor: (p) => pesos(p.precio_venta) },
      { titulo: 'Stock / mín.', clase: 'derecha num', valor: (p) => `${numero(p.stock)} / ${numero(p.stock_minimo)}` },
    ],
    formulario: async (p) => {
      const [rc, rp, re, rf] = await Promise.all([get('/api/categorias'), get('/api/proveedores'), get('/api/etiquetas'), p ? get('/api/productos/' + p.id) : Promise.resolve({ ok: true, data: null })]);
      if (!rc.ok || !rp.ok || !re.ok || !rf.ok) return aviso('No se pudieron cargar los datos del formulario: ' + [rc, rp, re, rf].find((r) => !r.ok).error, 'error');
      const f = rf.data?.ficha || {};
      const propias = new Set((rf.data?.etiquetas || []).map((e) => e.id));
      const entradaNum = (name, valor, attrs = {}) => h('input', { name, type: 'number', min: '0', value: valor ?? '', inputmode: 'decimal', ...attrs });
      const area = (name, valor) => h('textarea', { name, rows: '3' }, valor || '');
      const grupos = TIPOS_ETIQUETA.map(([t, titulo]) => {
        const lista = re.data.filter((e) => e.tipo === t);
        if (!lista.length) return null;
        return h('fieldset', { style: { border: '1px solid var(--color-borde)', borderRadius: '12px', padding: '12px 16px' } }, h('legend', { class: 'texto-peq negrita' }, titulo),
          h('div', { class: 'fila' }, lista.map((e) => h('label', { class: 'check' }, h('input', { type: 'checkbox', name: 'etiquetas', value: e.id, checked: propias.has(e.id) }), e.nombre))));
      });
      return h('div', { class: 'pila' },
        h('div', { class: 'rejilla-form' },
          campo('Nombre', h('input', { name: 'nombre', required: true, maxlength: '150', value: f.nombre || '' }), { clase: 'ancho-total' }),
          campo('Marca', h('input', { name: 'marca', maxlength: '100', value: f.marca || '' })),
          campo('Presentación', h('input', { name: 'presentacion', maxlength: '80', value: f.presentacion || '' })),
          campo('Código de barras', h('input', { name: 'codigo_barras', maxlength: '50', value: f.codigo_barras || '' })),
          campo('Categoría', selector([{ valor: '', texto: 'Seleccione' }, ...rc.data.map((c) => ({ valor: c.id, texto: c.nombre, seleccionado: c.id === f.categoria_id }))], { name: 'categoria_id', required: true })),
          campo('Proveedor', selector([{ valor: '', texto: 'Sin proveedor' }, ...rp.data.map((x) => ({ valor: x.id, texto: x.nombre, seleccionado: x.id === f.proveedor?.id }))], { name: 'proveedor_id' })),
          campo('Precio de venta', entradaNum('precio_venta', f.precio_venta ?? 0, { step: '0.01', required: true })),
          campo('Margen %', entradaNum('margen_porcentaje', f.margen_porcentaje, { step: '0.01' })),
          campo('Stock mínimo', entradaNum('stock_minimo', f.stock_minimo ?? 0, { step: '1', required: true })),
          campo('Días alerta temprana', entradaNum('dias_alerta_temprana', f.dias_alerta_temprana ?? 120, { step: '1', required: true }), { ayuda: 'Empieza a avisar antes del vencimiento' }),
          campo('Días para liquidar', entradaNum('dias_alerta_liquidacion', f.dias_alerta_liquidacion ?? 60, { step: '1', required: true })),
          interruptorCampo('es_granel', 'Se vende a granel (100 g por unidad)', !!f.es_granel)),
        campo('Para qué sirve', area('para_que_sirve', f.para_que_sirve)),
        campo('Modo de uso', area('modo_toma', f.modo_toma)),
        campo('Contraindicaciones', area('contraindicaciones', f.contraindicaciones)),
        campo('Ingredientes', area('ingredientes', f.ingredientes)),
        campo('Aviso de complemento', h('input', { name: 'aviso_complemento', required: true, maxlength: '255', value: f.aviso_complemento || 'Es un complemento, no un medicamento. Ante algo serio, consulte a su médico.' })),
        grupos.some(Boolean) ? h('div', { class: 'pila-1' }, h('span', { class: 'texto-peq negrita' }, 'Etiquetas'), grupos) : null);
    },
    guardar: (form, p) => {
      const e = form.elements;
      const num = (v) => (String(v).trim() === '' ? null : Number(v));
      return (p ? put : post)('/api/productos' + (p ? '/' + p.id : ''), {
        nombre: e.nombre.value, marca: nulo(e.marca.value), presentacion: nulo(e.presentacion.value), codigo_barras: nulo(e.codigo_barras.value),
        categoria_id: Number(e.categoria_id.value), proveedor_id: num(e.proveedor_id.value), precio_venta: Number(e.precio_venta.value), margen_porcentaje: num(e.margen_porcentaje.value),
        stock_minimo: Number(e.stock_minimo.value), dias_alerta_temprana: Number(e.dias_alerta_temprana.value), dias_alerta_liquidacion: Number(e.dias_alerta_liquidacion.value),
        es_granel: e.es_granel.value === '1', para_que_sirve: nulo(e.para_que_sirve.value), modo_toma: nulo(e.modo_toma.value), contraindicaciones: nulo(e.contraindicaciones.value),
        ingredientes: nulo(e.ingredientes.value), aviso_complemento: e.aviso_complemento.value,
        etiquetas: [...form.querySelectorAll('input[name="etiquetas"]:checked')].map((c) => Number(c.value)),
      });
    },
    eliminar: (p) => del('/api/productos/' + p.id),
  });
}

export default function catalogo(contenedor) {
  const gestiona = (r) => puede(`${r}.crear`, `${r}.editar`, `${r}.eliminar`);
  const lista = [
    gestiona('productos') && { id: 'productos', titulo: 'Productos', icono: 'package', crear: productosCrud },
    gestiona('categorias') && { id: 'categorias', titulo: 'Categorías', icono: 'tags', crear: categorias },
    gestiona('proveedores') && { id: 'proveedores', titulo: 'Proveedores', icono: 'truck', crear: proveedores },
    gestiona('etiquetas') && { id: 'etiquetas', titulo: 'Etiquetas', icono: 'leaf', crear: etiquetas },
  ].filter(Boolean);
  const area = h('div', { role: 'tabpanel' }, esqueleto());
  const mostrar = (id) => { area.setAttribute('aria-labelledby', 'tab-' + id); area.replaceChildren(lista.find((x) => x.id === id).crear()); };
  const { barra } = pestanas(lista, { activa: lista[0].id, alCambiar: mostrar });
  contenedor.append(
    h('div', { class: 'cabecera-pagina' }, h('div', {}, h('h2', {}, 'Catálogo'), h('p', {}, 'Administre la información que usa todo el equipo.'))),
    barra, area);
  mostrar(lista[0].id);
}
