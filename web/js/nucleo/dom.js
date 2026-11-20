// Utilidades de DOM y formato. Todo texto entra como textContent (nunca innerHTML),
// así los datos de la base no pueden inyectar HTML.

const PROPIEDADES = new Set(['value', 'checked', 'selected', 'indeterminate', 'disabled', 'hidden', 'open']);

export function h(tag, attrs, ...hijos) {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v === null || v === undefined || v === false) continue;
    if (k === 'class') el.className = v;
    else if (k === 'style' && typeof v === 'object') Object.assign(el.style, v);
    else if (k === 'dataset') Object.assign(el.dataset, v);
    else if (k.startsWith('on') && typeof v === 'function') el.addEventListener(k.slice(2), v);
    else if (PROPIEDADES.has(k)) el[k] = v;
    else el.setAttribute(k, v === true ? '' : v);
  }
  agregar(el, hijos);
  return el;
}

export function agregar(el, hijos) {
  for (const hijo of [hijos].flat(Infinity)) {
    if (hijo === null || hijo === undefined || hijo === false || hijo === '') continue;
    el.append(hijo instanceof Node ? hijo : String(hijo));
  }
  return el;
}

export function reemplazar(el, ...hijos) {
  el.replaceChildren();
  return agregar(el, hijos);
}

export const $ = (sel, raiz = document) => raiz.querySelector(sel);
export const $$ = (sel, raiz = document) => [...raiz.querySelectorAll(sel)];

let contadorIds = 0;
export const nuevoId = (prefijo = 'gm') => `${prefijo}-${++contadorIds}`;

// --- Formato (Colombia) ---
const fmtPesos = new Intl.NumberFormat('es-CO', { style: 'currency', currency: 'COP', maximumFractionDigits: 0 });
const fmtNumero = new Intl.NumberFormat('es-CO');
export const pesos = (n) => fmtPesos.format(Number(n) || 0);
export const numero = (n) => fmtNumero.format(Number(n) || 0);
export const texto = (v) => (v === null || v === undefined || v === '' ? '—' : String(v));

// "AAAA-MM-DD" → "DD/MM/AAAA" sin pasar por Date (evita corrimientos de zona horaria).
export function fechaCO(iso) {
  if (!iso) return '—';
  const [a, m, d] = String(iso).slice(0, 10).split('-');
  return d && m && a ? `${d}/${m}/${a}` : String(iso);
}
export function fechaHora(iso) {
  if (!iso) return '—';
  const s = String(iso);
  return `${fechaCO(s)} ${s.slice(11, 16)}`.trim();
}

const ZONA = 'America/Bogota';
export const hoyISO = () => new Date().toLocaleDateString('en-CA', { timeZone: ZONA });
export function haceDiasISO(dias) {
  const d = new Date();
  d.setDate(d.getDate() - dias);
  return d.toLocaleDateString('en-CA', { timeZone: ZONA });
}
export const fechaLarga = () => new Date().toLocaleDateString('es-CO', { timeZone: ZONA, weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' });
export function saludo() {
  const hora = Number(new Date().toLocaleString('en-US', { timeZone: ZONA, hour: 'numeric', hour12: false }));
  return hora < 12 ? 'Buenos días' : hora < 19 ? 'Buenas tardes' : 'Buenas noches';
}
export const iniciales = (nombre) => String(nombre || '?').trim().split(/\s+/).slice(0, 2).map((p) => p[0]).join('').toUpperCase();

export function debounce(fn, ms = 300) {
  let t;
  return (...args) => { clearTimeout(t); t = setTimeout(() => fn(...args), ms); };
}

export const normalizar = (s) => String(s || '').normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase();
