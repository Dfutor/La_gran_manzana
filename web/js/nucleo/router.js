// Enrutador con History API. Las rutas viven bajo /granmanzana; el servidor
// responde index.html para cualquier ruta de la aplicación.

export const BASE = '/granmanzana';
export const eventosRuta = new EventTarget();

export const rutaActual = () => location.pathname.slice(BASE.length).replace(/\/+$/, '') || '/';
export const parametros = () => new URLSearchParams(location.search);
export const url = (ruta) => BASE + ruta;

export function navegar(ruta, { reemplazar = false } = {}) {
  history[reemplazar ? 'replaceState' : 'pushState'](null, '', BASE + ruta);
  eventosRuta.dispatchEvent(new Event('cambio'));
}

// Compara '/admin/usuarios/:id' con '/admin/usuarios/7' → {id: '7'} o null.
export function coincide(patron, ruta) {
  const a = patron.split('/').filter(Boolean);
  const b = ruta.split('/').filter(Boolean);
  if (a.length !== b.length) return null;
  const params = {};
  for (let i = 0; i < a.length; i++) {
    if (a[i].startsWith(':')) params[a[i].slice(1)] = decodeURIComponent(b[i]);
    else if (a[i] !== b[i]) return null;
  }
  return params;
}

// Los enlaces internos con data-enlace navegan sin recargar la página.
document.addEventListener('click', (e) => {
  const a = e.target.closest('a[data-enlace]');
  if (!a || e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
  const destino = new URL(a.href);
  if (destino.origin !== location.origin || !destino.pathname.startsWith(BASE)) return;
  e.preventDefault();
  navegar(destino.pathname.slice(BASE.length) + destino.search);
});
window.addEventListener('popstate', () => eventosRuta.dispatchEvent(new Event('cambio')));
