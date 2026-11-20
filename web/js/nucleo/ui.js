// Componentes de interfaz reutilizables.
import { h, agregar, nuevoId, $ } from './dom.js';
import { icono } from './iconos.js';

// --- Botones ---
export function boton(contenido, { tipo = 'secundario', icono: ic, peq = false, submit = false, ...attrs } = {}) {
  const clases = ['btn', `btn-${tipo}`, peq ? 'btn-peq' : '', attrs.class || ''].filter(Boolean).join(' ');
  delete attrs.class;
  return h('button', { type: submit ? 'submit' : 'button', class: clases, ...attrs }, ic ? icono(ic, { tam: peq ? 16 : 18 }) : null, contenido);
}
export function botonIcono(nombreIcono, etiqueta, { class: clase = '', ...attrs } = {}) {
  return h('button', { type: 'button', 'aria-label': etiqueta, title: etiqueta, ...attrs, class: ('btn btn-fantasma btn-icono ' + clase).trim() },
    icono(nombreIcono, { tam: 18 }));
}
// Pone el botón en estado «cargando» mientras corre la promesa.
export async function conCarga(btn, promesa) {
  const original = [...btn.childNodes];
  btn.disabled = true;
  btn.setAttribute('aria-busy', 'true');
  btn.replaceChildren(icono('refresh-cw', { tam: 16, clase: 'girando' }), 'Procesando…');
  try { return await promesa; } finally {
    btn.disabled = false;
    btn.removeAttribute('aria-busy');
    btn.replaceChildren(...original);
  }
}

// --- Campos ---
export function campo(etiqueta, control, { ayuda, clase = '' } = {}) {
  const id = control.id || nuevoId('campo');
  control.id = id;
  const partes = [h('label', { for: id }, etiqueta), control];
  if (ayuda) {
    const idAyuda = id + '-ayuda';
    control.setAttribute('aria-describedby', idAyuda);
    partes.push(h('span', { class: 'ayuda', id: idAyuda }, ayuda));
  }
  return h('div', { class: ('campo ' + clase).trim() }, partes);
}
export const entrada = (attrs = {}) => h('input', { type: 'text', ...attrs });
export function selector(opciones, attrs = {}) {
  return h('select', attrs, opciones.map((o) => (o instanceof Node ? o : h('option', { value: o.valor, selected: o.seleccionado || false, disabled: o.deshabilitado || false }, o.texto))));
}

// Interruptor accesible: <button role="switch" aria-checked>.
export function interruptor({ activo = false, etiqueta, deshabilitado = false, alCambiar, ...attrs } = {}) {
  const btn = h('button', {
    type: 'button', role: 'switch', class: 'interruptor', 'aria-checked': String(!!activo),
    'aria-label': etiqueta, disabled: deshabilitado, ...attrs,
  });
  btn.addEventListener('click', () => {
    const nuevo = btn.getAttribute('aria-checked') !== 'true';
    btn.setAttribute('aria-checked', String(nuevo));
    alCambiar?.(nuevo);
  });
  return btn;
}
export function interruptorConEtiqueta(texto, opciones) {
  const sw = interruptor({ ...opciones, etiqueta: undefined });
  const id = nuevoId('sw');
  sw.setAttribute('aria-labelledby', id);
  const etq = h('span', { id }, texto);
  return { elemento: h('div', { class: 'interruptor-etiqueta', onclick: (e) => { if (e.target === etq) sw.click(); } }, sw, etq), sw };
}

// Control segmentado con radios nativos (accesible por teclado con flechas).
export function segmentado(nombre, opciones, valor, { alCambiar, etiqueta, deshabilitado = false } = {}) {
  const grupo = h('div', { class: 'segmentado', role: 'radiogroup', 'aria-label': etiqueta });
  for (const o of opciones) {
    const input = h('input', { type: 'radio', name: nombre, value: o.valor, checked: o.valor === valor, disabled: deshabilitado || o.deshabilitado || false });
    input.addEventListener('change', () => alCambiar?.(o.valor));
    grupo.append(h('label', { class: o.clase || '', title: o.titulo || '' }, input, h('span', {}, o.texto)));
  }
  return grupo;
}

// --- Insignias, avisos, estados ---
export const insignia = (contenido, tipo = '', ic) => h('span', { class: 'insignia' + (tipo ? ' insignia-' + tipo : '') }, ic ? icono(ic, { tam: 12 }) : null, contenido);

export function aviso(contenido, tipo = 'info') {
  const iconos = { info: 'info', exito: 'circle-check', advertencia: 'circle-alert', error: 'circle-alert' };
  return h('div', { class: `aviso aviso-${tipo}`, role: tipo === 'error' ? 'alert' : null }, icono(iconos[tipo] || 'info', { tam: 18 }), h('div', {}, contenido));
}

export function vacio({ icono: ic = 'circle-check', titulo, texto, accion } = {}) {
  return h('div', { class: 'vacio' }, h('div', { class: 'vacio-icono' }, icono(ic, { tam: 26 })), h('h3', {}, titulo), texto ? h('p', {}, texto) : null, accion || null);
}

export function esqueleto({ lineas = 3, alto = 14, ancho } = {}) {
  return h('div', { class: 'pila-1', 'aria-hidden': 'true' },
    Array.from({ length: lineas }, (_, i) => h('span', { class: 'esqueleto', style: { height: alto + 'px', width: ancho || (i === lineas - 1 ? '60%' : '100%') } })));
}
export const cargando = (texto = 'Cargando…') => h('div', { role: 'status', class: 'solo-lectores' }, texto);
export function esqueletoTabla(filas = 5) {
  return h('div', { class: 'pila-1', style: { padding: '16px' }, 'aria-busy': 'true' }, cargando(),
    Array.from({ length: filas }, () => h('span', { class: 'esqueleto', style: { height: '36px' } })));
}

export function errorCarga(r, reintentar) {
  return h('div', { class: 'pila', style: { padding: '16px' } }, aviso(r.error || 'No se pudo cargar la información.', 'error'),
    reintentar ? h('div', {}, boton('Reintentar', { icono: 'refresh-cw', peq: true, onclick: reintentar })) : null);
}

// --- Notificaciones flotantes ---
export function notificar(mensaje, tipo = 'info', ms = 4500) {
  const caja = $('#avisos');
  const iconos = { info: 'info', exito: 'circle-check', error: 'circle-alert' };
  const t = h('div', { class: `toast toast-${tipo}` }, icono(iconos[tipo] || 'info', { tam: 18 }), h('div', {}, mensaje));
  caja.append(t);
  setTimeout(() => t.remove(), ms);
}

// --- Diálogos ---
// modal({titulo, contenido, acciones:[boton...], ancho}) devuelve {dialogo, cerrar}.
export function modal({ titulo, descripcion, contenido, acciones = [], ancho = false, panel = false, alCerrar } = {}) {
  const idTitulo = nuevoId('titulo');
  const dialogo = h('dialog', { class: panel ? 'panel' : 'modal' + (ancho ? ' modal-ancho' : ''), 'aria-labelledby': idTitulo });
  const cerrar = () => { if (dialogo.open) dialogo.close(); };
  dialogo.append(
    h('div', { class: 'modal-cabecera' },
      h('div', {}, h('h2', { id: idTitulo, style: { fontSize: '20px' } }, titulo), descripcion ? h('p', { class: 'texto-2 texto-peq', style: { marginTop: '4px' } }, descripcion) : null),
      botonIcono('x', 'Cerrar', { onclick: cerrar })),
    h('div', { class: 'modal-cuerpo' }, contenido),
    acciones.length ? h('div', { class: 'modal-pie' }, acciones) : null);
  dialogo.addEventListener('close', () => { dialogo.remove(); alCerrar?.(); });
  dialogo.addEventListener('click', (e) => { if (e.target === dialogo) cerrar(); });
  document.body.append(dialogo);
  dialogo.showModal();
  return { dialogo, cerrar };
}

export function confirmar({ titulo, mensaje, confirmar: textoOk = 'Confirmar', peligro = false }) {
  return new Promise((resolver) => {
    let respuesta = false;
    const ok = boton(textoOk, { tipo: peligro ? 'peligro' : 'primario' });
    const cancelar = boton('Cancelar');
    const m = modal({ titulo, contenido: typeof mensaje === 'string' ? h('p', {}, mensaje) : mensaje, acciones: [cancelar, ok], alCerrar: () => resolver(respuesta) });
    ok.addEventListener('click', () => { respuesta = true; m.cerrar(); });
    cancelar.addEventListener('click', m.cerrar);
    setTimeout(() => cancelar.focus(), 0);
  });
}

// Formulario dentro de un modal. enviar(form) devuelve {ok, error}; si ok, se cierra.
export function modalFormulario({ titulo, descripcion, campos, textoEnviar = 'Guardar', enviar, ancho = false }) {
  const errorCaja = h('div', { 'aria-live': 'assertive' });
  const form = h('form', { class: 'pila', novalidate: true }, errorCaja, campos);
  const btnEnviar = boton(textoEnviar, { tipo: 'primario', submit: true, form: nuevoId('f') });
  form.id = btnEnviar.getAttribute('form');
  const cancelar = boton('Cancelar');
  const m = modal({ titulo, descripcion, contenido: form, acciones: [cancelar, btnEnviar], ancho });
  cancelar.addEventListener('click', m.cerrar);
  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    errorCaja.replaceChildren();
    if (!form.checkValidity()) { form.reportValidity(); return; }
    const r = await conCarga(btnEnviar, enviar(form));
    if (r && r.ok === false) errorCaja.replaceChildren(aviso(r.error, 'error'));
    else m.cerrar();
  });
  setTimeout(() => form.querySelector('input,select,textarea')?.focus(), 0);
  return m;
}

// --- Pestañas accesibles (role=tablist) ---
export function pestanas(lista, { activa, alCambiar } = {}) {
  const barra = h('div', { class: 'pestanas', role: 'tablist' });
  const botones = lista.map((p) => h('button', {
    type: 'button', role: 'tab', id: 'tab-' + p.id, 'aria-selected': String(p.id === activa), tabindex: p.id === activa ? '0' : '-1',
    onclick: () => seleccionar(p.id),
  }, p.icono ? icono(p.icono, { tam: 18 }) : null, p.titulo));
  function seleccionar(id) {
    botones.forEach((b, i) => { const sel = lista[i].id === id; b.setAttribute('aria-selected', String(sel)); b.tabIndex = sel ? 0 : -1; });
    alCambiar?.(id);
  }
  barra.addEventListener('keydown', (e) => {
    const i = botones.indexOf(document.activeElement);
    if (i < 0 || !['ArrowRight', 'ArrowLeft'].includes(e.key)) return;
    const j = (i + (e.key === 'ArrowRight' ? 1 : -1) + botones.length) % botones.length;
    botones[j].focus();
    seleccionar(lista[j].id);
  });
  agregar(barra, botones);
  return { barra, seleccionar };
}

// --- Tablas ---
// tabla(columnas, filas, {alClic}) con columnas [{titulo, valor:(fila)=>nodo|texto, clase}]
export function tabla(columnas, filas, { alClic, etiqueta } = {}) {
  return h('div', { class: 'tabla-contenedor' }, h('table', { class: 'tabla', 'aria-label': etiqueta },
    h('thead', {}, h('tr', {}, columnas.map((c) => h('th', { class: c.clase || '', scope: 'col' }, c.titulo)))),
    h('tbody', {}, filas.map((f) => {
      const tr = h('tr', alClic ? { class: 'clicable', tabindex: '0', onclick: () => alClic(f), onkeydown: (e) => { if (e.key === 'Enter') alClic(f); } } : {},
        columnas.map((c) => h('td', { class: c.clase || '' }, c.valor(f))));
      return tr;
    }))));
}

// Paginación local: devuelve {pie, pagina(lista)}.
export function paginador(tamano, alCambiar) {
  let pagina = 0;
  let total = 0;
  const info = h('span', { 'aria-live': 'polite' });
  const ant = boton('Anterior', { peq: true, icono: 'chevron-right', onclick: () => { pagina--; alCambiar(); } });
  ant.querySelector('svg').style.transform = 'rotate(180deg)';
  const sig = boton('Siguiente', { peq: true, onclick: () => { pagina++; alCambiar(); } });
  sig.append(icono('chevron-right', { tam: 16 }));
  const pie = h('div', { class: 'paginacion' }, info, h('div', { class: 'fila' }, ant, sig));
  return {
    pie,
    reiniciar: () => { pagina = 0; },
    pagina(lista) {
      total = lista.length;
      const paginas = Math.max(1, Math.ceil(total / tamano));
      pagina = Math.min(Math.max(0, pagina), paginas - 1);
      const desde = pagina * tamano;
      info.textContent = total ? `${desde + 1}–${Math.min(desde + tamano, total)} de ${total}` : 'Sin resultados';
      ant.disabled = pagina === 0;
      sig.disabled = pagina >= paginas - 1;
      pie.hidden = total <= tamano;
      return lista.slice(desde, desde + tamano);
    },
  };
}
