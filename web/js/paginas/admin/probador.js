// Probador de API: envía peticiones con la sesión actual (mismos permisos del usuario).
import { h } from '../../nucleo/dom.js';
import { api } from '../../nucleo/api.js';
import { boton, campo, selector, conCarga, insignia } from '../../nucleo/ui.js';
import { url } from '../../nucleo/router.js';

const EJEMPLOS = [
  ['GET', '/api/salud'], ['GET', '/api/sesion'], ['GET', '/api/productos?q='], ['GET', '/api/productos/1/plan-fefo?cantidad=5'],
  ['GET', '/api/sugerencias'], ['GET', '/api/roles'], ['GET', '/api/permisos'], ['GET', '/api/auditoria?limite=10'],
  ['POST', '/api/categorias', '{\n  "nombre": "Categoría de prueba"\n}'],
];

export default function probador(contenedor) {
  const metodo = selector(['GET', 'POST', 'PUT', 'DELETE'].map((m) => ({ valor: m, texto: m })), { 'aria-label': 'Método' });
  const ruta = h('input', { value: '/api/salud', spellcheck: 'false', class: 'mono' });
  const cuerpo = h('textarea', { rows: '6', spellcheck: 'false', class: 'mono', placeholder: '{ }' });
  const estado = h('div', { class: 'fila' });
  const salida = h('pre', { class: 'codigo', 'aria-live': 'polite' }, 'La respuesta aparecerá aquí.');
  const enviar = boton('Enviar', { tipo: 'primario', icono: 'arrow-right' });
  enviar.addEventListener('click', async () => {
    let r0 = ruta.value.trim();
    if (!r0.startsWith('/')) r0 = '/' + r0;
    const inicio = performance.now();
    const r = await conCarga(enviar, api(r0, { metodo: metodo.value, cuerpo: ['POST', 'PUT'].includes(metodo.value) && cuerpo.value.trim() ? cuerpo.value : undefined }));
    estado.replaceChildren(insignia(r.status ? 'HTTP ' + r.status : 'Sin respuesta', r.ok ? 'exito' : 'error'), h('span', { class: 'texto-2 texto-peq' }, `${Math.round(performance.now() - inicio)} ms`));
    salida.textContent = r.data !== null ? JSON.stringify(r.data, null, 2) : (r.crudo || r.error || '(sin cuerpo)');
  });
  contenedor.append(
    h('div', { class: 'cabecera-pagina' }, h('div', {}, h('h2', {}, 'Probador de API'), h('p', {}, 'Las peticiones usan su sesión: solo funcionan las rutas que sus permisos permiten. Documentación completa en ', h('a', { href: url('/docs/swagger.html'), target: '_blank', rel: 'noopener' }, 'Swagger'), '.'))),
    h('div', { class: 'columnas-2' },
      h('div', { class: 'tarjeta pila' },
        h('div', { class: 'rejilla-form', style: { gridTemplateColumns: '120px 1fr' } }, campo('Método', metodo), campo('Ruta', ruta)),
        campo('Cuerpo JSON (POST/PUT)', cuerpo), h('div', {}, enviar),
        h('div', { class: 'pila-1' }, h('span', { class: 'texto-peq negrita' }, 'Ejemplos'), h('div', { class: 'fila' }, EJEMPLOS.map(([m, r, c]) => h('button', {
          type: 'button', class: 'chip', onclick: () => { metodo.value = m; ruta.value = r; cuerpo.value = c || ''; },
        }, `${m} ${r}`))))),
      h('div', { class: 'tarjeta pila' }, h('div', { class: 'fila-entre' }, h('h3', {}, 'Respuesta'), estado), salida)));
}
