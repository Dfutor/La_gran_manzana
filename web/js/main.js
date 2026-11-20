// Punto de entrada: rutas, guardas de acceso y renderizado de páginas.
import { h } from './nucleo/dom.js';
import { icono } from './nucleo/iconos.js';
import { eventosRuta, rutaActual, navegar, coincide, parametros, url } from './nucleo/router.js';
import { cargarSesion, haySesion, puede, esAdmin, eventosSesion } from './nucleo/sesion.js';
import { MODULOS, MODULOS_ADMIN } from './nucleo/modulos.js';
import { montarShell, shellMontado, prepararPagina, desmontarShell, modulosVisibles } from './nucleo/shell.js';
import { notificar, vacio } from './nucleo/ui.js';

const modulo = (id) => [...MODULOS, ...MODULOS_ADMIN].find((m) => m.id === id);

// acceso: 'publico' | 'sesion' | 'admin'. El permiso sale del módulo.
const RUTAS = [
  { patron: '/', acceso: 'publico', cargar: () => import('./paginas/landing.js') },
  { patron: '/login', acceso: 'publico', cargar: () => import('./paginas/login.js'), opciones: { variante: 'usuario' } },
  { patron: '/admin/login', acceso: 'publico', cargar: () => import('./paginas/login.js'), opciones: { variante: 'admin' } },
  { patron: '/app', acceso: 'sesion', modulo: 'inicio', cargar: () => import('./paginas/dashboard.js') },
  { patron: '/app/productos', acceso: 'sesion', modulo: 'productos', cargar: () => import('./paginas/productos.js') },
  { patron: '/app/consulta', acceso: 'sesion', modulo: 'consulta', cargar: () => import('./paginas/consulta.js') },
  { patron: '/app/ventas', acceso: 'sesion', modulo: 'ventas', cargar: () => import('./paginas/ventas.js') },
  { patron: '/app/inventario', acceso: 'sesion', modulo: 'inventario', cargar: () => import('./paginas/inventario.js') },
  { patron: '/app/alertas', acceso: 'sesion', modulo: 'alertas', cargar: () => import('./paginas/alertas.js') },
  { patron: '/app/catalogo', acceso: 'sesion', modulo: 'catalogo', cargar: () => import('./paginas/catalogo.js') },
  { patron: '/app/reportes', acceso: 'sesion', modulo: 'reportes', cargar: () => import('./paginas/reportes.js') },
  { patron: '/admin', acceso: 'admin', redirigir: () => (modulosVisibles(MODULOS_ADMIN)[0]?.ruta || '/app') },
  { patron: '/admin/usuarios', acceso: 'admin', modulo: 'usuarios', cargar: () => import('./paginas/admin/usuarios.js') },
  { patron: '/admin/usuarios/:id', acceso: 'admin', modulo: 'usuarios', cargar: () => import('./paginas/admin/usuario-permisos.js') },
  { patron: '/admin/roles', acceso: 'admin', modulo: 'roles', cargar: () => import('./paginas/admin/roles.js') },
  { patron: '/admin/auditoria', acceso: 'admin', modulo: 'auditoria', cargar: () => import('./paginas/admin/auditoria.js') },
  { patron: '/admin/probador', acceso: 'admin', modulo: 'probador', cargar: () => import('./paginas/admin/probador.js') },
];

const raiz = document.getElementById('raiz');
let turno = 0;

function sinAcceso(contenedor) {
  contenedor.append(h('div', { class: 'tarjeta' }, vacio({
    icono: 'lock', titulo: 'No tiene acceso a esta sección',
    texto: 'Sus roles no incluyen este módulo. Si lo necesita, pídale a un administrador que se lo asigne.',
    accion: h('a', { class: 'btn btn-primario', href: url('/app'), 'data-enlace': '' }, 'Volver al inicio'),
  })));
}

async function renderizar() {
  const mio = ++turno;
  const ruta = rutaActual();
  let destino = null;
  let params = null;
  for (const r of RUTAS) {
    params = coincide(r.patron, ruta);
    if (params) { destino = r; break; }
  }
  if (!destino) { navegar(haySesion() ? '/app' : '/', { reemplazar: true }); return; }

  if (destino.acceso !== 'publico' && !haySesion()) {
    const login = destino.acceso === 'admin' ? '/admin/login' : '/login';
    navegar(`${login}?siguiente=${encodeURIComponent(ruta + location.search)}`, { reemplazar: true });
    return;
  }
  if (destino.patron === '/login' && haySesion()) { navegar('/app', { reemplazar: true }); return; }
  if (destino.redirigir) { navegar(destino.redirigir(), { reemplazar: true }); return; }

  const ctx = { params, query: parametros(), opciones: destino.opciones || {} };
  if (destino.acceso === 'publico') {
    desmontarShell();
    const pagina = await destino.cargar();
    if (mio !== turno) return;
    raiz.replaceChildren();
    await pagina.default(raiz, ctx);
    window.scrollTo(0, 0);
    return;
  }

  if (!shellMontado()) montarShell(raiz);
  const m = modulo(destino.modulo);
  const contenedor = prepararPagina(m.titulo, m.id);
  if (m.permisos.length && !puede(...m.permisos)) { sinAcceso(contenedor); return; }
  if (destino.acceso === 'admin' && !esAdmin()) { sinAcceso(contenedor); return; }
  const pagina = await destino.cargar();
  if (mio !== turno) return;
  try {
    await pagina.default(contenedor, ctx);
  } catch (err) {
    console.error(err);
    contenedor.replaceChildren(h('div', { class: 'aviso aviso-error', role: 'alert' }, icono('circle-alert', { tam: 18 }), 'Ocurrió un error al mostrar esta página. Recárguela e intente de nuevo.'));
  }
  contenedor.focus({ preventScroll: true });
  window.scrollTo(0, 0);
}

eventosRuta.addEventListener('cambio', renderizar);

// Si cambian los roles o permisos (o vence la sesión), se reconstruye la navegación.
eventosSesion.addEventListener('cambio', (e) => {
  const enAplicacion = rutaActual().startsWith('/app') || (rutaActual().startsWith('/admin') && rutaActual() !== '/admin/login');
  if (!haySesion()) {
    desmontarShell();
    if (enAplicacion && !e.detail?.voluntario) {
      notificar('Su sesión terminó. Ingrese de nuevo.', 'info');
      navegar(`/login?siguiente=${encodeURIComponent(rutaActual())}`, { reemplazar: true });
    }
    return;
  }
  if (shellMontado()) {
    desmontarShell();
    notificar('Sus permisos cambiaron; la pantalla se actualizó.', 'info');
    renderizar();
  }
});

// Revisa cada minuto si un administrador cambió los permisos de esta sesión.
setInterval(() => { if (haySesion()) import('./nucleo/sesion.js').then((s) => s.refrescarSesion()); }, 60000);

// Con <base href>, un enlace "#contenido" navegaría a otra página: se enfoca a mano.
document.querySelector('.saltar').addEventListener('click', (e) => {
  e.preventDefault();
  document.getElementById('contenido')?.focus();
});

await cargarSesion();
renderizar();
