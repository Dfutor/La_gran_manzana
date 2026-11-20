// Permisos de un usuario: roles asignados, personalización por permiso
// (heredado del rol / conceder / denegar) y permisos efectivos resultantes.
// Prioridad: override del usuario > permisos del rol > denegado.
import { h, iniciales } from '../../nucleo/dom.js';
import { icono } from '../../nucleo/iconos.js';
import { get, put } from '../../nucleo/api.js';
import { puede, usuarioActual } from '../../nucleo/sesion.js';
import { url } from '../../nucleo/router.js';
import { insignia, aviso, esqueleto, errorCarga, boton, notificar, segmentado, confirmar, conCarga, vacio } from '../../nucleo/ui.js';

export default async function usuarioPermisos(contenedor, { params }) {
  const id = Number(params.id);
  const yo = usuarioActual();
  const cuerpo = h('div', {}, esqueleto({ lineas: 8, alto: 40 }));
  contenedor.append(h('p', { style: { marginBottom: '16px' } }, h('a', { href: url('/admin/usuarios'), 'data-enlace': '' }, '← Volver a usuarios')), cuerpo);

  async function cargar() {
    const [r, rr] = await Promise.all([get(`/api/usuarios/${id}/permisos`), puede('roles.ver') ? get('/api/roles') : Promise.resolve({ ok: true, data: [] })]);
    if (!r.ok) { cuerpo.replaceChildren(errorCarga(r, cargar)); return; }
    pintar(r.data, rr.ok ? rr.data : []);
  }

  function pintar(datos, roles) {
    const u = datos.usuario;
    const propio = u.id === yo.id;
    const asigna = puede('roles.asignar') && !propio;
    const propios = new Set(yo.permisos);

    // --- Roles ---
    const rolesActuales = new Set(datos.roles.map((r) => r.id));
    const seleccion = new Set(rolesActuales);
    const guardarRoles = boton('Guardar roles', { tipo: 'primario', peq: true, icono: 'check', disabled: true });
    const casillasRoles = roles.map((r) => {
      const c = h('input', { type: 'checkbox', checked: rolesActuales.has(r.id), disabled: !asigna });
      c.addEventListener('change', () => {
        if (c.checked) seleccion.add(r.id); else seleccion.delete(r.id);
        const cambio = seleccion.size !== rolesActuales.size || [...seleccion].some((x) => !rolesActuales.has(x));
        guardarRoles.disabled = !cambio;
      });
      return h('label', { class: 'check', style: { padding: '8px 12px', border: '1px solid var(--color-borde)', borderRadius: '8px' } }, c,
        h('span', {}, h('span', { class: 'negrita' }, r.nombre), ' ', h('span', { class: 'texto-2 texto-mini' }, `${r.permisos.length} permisos${r.activo ? '' : ' · inactivo'}${r.es_sistema ? ' · sistema' : ''}`)));
    });
    guardarRoles.addEventListener('click', async () => {
      const rr = await conCarga(guardarRoles, put(`/api/usuarios/${id}/roles`, { roles: [...seleccion] }));
      notificar(rr.ok ? 'Roles actualizados. Aplican en la siguiente acción del usuario.' : rr.error, rr.ok ? 'exito' : 'error');
      if (rr.ok) cargar();
    });
    const tarjetaRoles = h('section', { class: 'tarjeta pila', 'aria-labelledby': 't-roles' },
      h('div', { class: 'fila-entre' }, h('h3', { id: 't-roles' }, 'Roles asignados'), asigna ? guardarRoles : null),
      roles.length ? h('div', { class: 'fila' }, casillasRoles) : h('div', { class: 'fila' }, datos.roles.length ? datos.roles.map((r) => insignia(r.nombre, 'exito')) : h('span', { class: 'texto-2' }, 'Sin roles')),
      propio ? aviso('No puede modificar sus propios roles ni permisos: otro administrador debe hacerlo.', 'info') : null,
      !puede('roles.asignar') ? aviso('Necesita el permiso «roles.asignar» para cambiar roles y permisos.', 'info') : null);

    // --- Personalización por permiso ---
    const original = new Map(datos.detalle.map((p) => [p.nombre, p.override || '']));
    const pendiente = new Map(original);
    const contador = h('span', {});
    const guardar = boton('Guardar cambios', { tipo: 'primario', icono: 'check' });
    const descartar = boton('Descartar', { icono: 'undo-2' });
    const barra = h('div', { class: 'barra-cambios', hidden: true, role: 'region', 'aria-label': 'Cambios pendientes' }, contador, h('div', { class: 'fila' }, descartar, guardar));
    const filas = new Map();

    function efectivoDe(p, valor) {
      if (valor === 'concedido') return true;
      if (valor === 'denegado') return datos.es_propietario ? true : false;
      return p.heredado;
    }
    function actualizarFila(p) {
      const valor = pendiente.get(p.nombre);
      const { fila, estado, origen } = filas.get(p.nombre);
      fila.classList.toggle('pendiente', valor !== original.get(p.nombre));
      const ef = efectivoDe(p, valor);
      estado.replaceChildren(ef ? insignia('Permitido', 'exito', 'check') : insignia('Denegado', 'error', 'ban'));
      origen.replaceChildren(valor
        ? insignia('Personalizado', 'info', 'pencil')
        : (p.heredado ? h('span', {}, 'Heredado de ', h('b', {}, p.roles.join(', ') || 'rol')) : h('span', {}, 'Heredado: sin permiso')));
      const cambios = [...pendiente].filter(([k, v]) => v !== original.get(k)).length;
      contador.textContent = `${cambios} permiso${cambios === 1 ? '' : 's'} con cambios sin guardar`;
      barra.hidden = !cambios;
    }

    const grupos = new Map();
    for (const p of datos.detalle) {
      if (!grupos.has(p.modulo)) grupos.set(p.modulo, []);
      grupos.get(p.modulo).push(p);
    }
    const listaGrupos = [...grupos].map(([modulo, lista]) => h('div', { class: 'grupo-permisos', role: 'group', 'aria-label': modulo },
      h('h4', {}, modulo),
      lista.map((p) => {
        const editable = asigna && propios.has(p.nombre);
        const estado = h('div', { style: { minWidth: '108px' } });
        const origen = h('div', { class: 'origen' });
        const control = segmentado('ov-' + p.nombre, [
          { valor: '', texto: 'Heredado', titulo: 'Usar el valor de sus roles' },
          { valor: 'concedido', texto: 'Conceder', clase: 'valor-conceder', titulo: 'Permitir aunque su rol no lo tenga' },
          { valor: 'denegado', texto: 'Denegar', clase: 'valor-denegar', titulo: 'Negar aunque su rol lo tenga', deshabilitado: datos.es_propietario },
        ], pendiente.get(p.nombre), {
          etiqueta: `Personalización de ${p.descripcion}`, deshabilitado: !editable,
          alCambiar: (v) => { pendiente.set(p.nombre, v); actualizarFila(p); },
        });
        const fila = h('div', { class: 'permiso-fila' },
          h('div', {}, h('div', { class: 'negrita' }, p.descripcion, p.critico ? h('span', { class: 'texto-mini', style: { color: 'var(--color-error)', marginLeft: '6px' } }, 'crítico') : null),
            h('code', { class: 'texto-2 texto-mini' }, p.nombre)),
          origen, control, estado);
        filas.set(p.nombre, { fila, estado, origen, control });
        return fila;
      })));

    const restablecer = boton('Restablecer todo al rol', { peq: true, icono: 'rotate-ccw', disabled: !asigna || !datos.overrides.length });
    restablecer.addEventListener('click', () => {
      for (const p of datos.detalle) {
        pendiente.set(p.nombre, '');
        filas.get(p.nombre).control.querySelector('input[value=""]').checked = true;
        actualizarFila(p);
      }
    });
    descartar.addEventListener('click', () => {
      for (const p of datos.detalle) {
        pendiente.set(p.nombre, original.get(p.nombre));
        filas.get(p.nombre).control.querySelector(`input[value="${original.get(p.nombre)}"]`).checked = true;
        actualizarFila(p);
      }
    });
    guardar.addEventListener('click', async () => {
      const cambios = [...pendiente].filter(([k, v]) => v !== original.get(k));
      const ok = await confirmar({
        titulo: 'Guardar permisos personalizados',
        mensaje: h('div', { class: 'pila-1' }, h('p', {}, `Se modificarán ${cambios.length} permisos de ${u.nombre}:`),
          h('ul', {}, cambios.map(([k, v]) => h('li', {}, h('code', {}, k), ' → ', v === 'concedido' ? 'conceder' : v === 'denegado' ? 'denegar' : 'heredar del rol')))),
        confirmar: 'Guardar',
      });
      if (!ok) return;
      const overrides = [...pendiente].filter(([, v]) => v).map(([permiso, v]) => ({ permiso, concedido: v === 'concedido' }));
      const r = await conCarga(guardar, put(`/api/usuarios/${id}/permisos`, { overrides }));
      notificar(r.ok ? 'Permisos guardados. Aplican en la siguiente acción del usuario.' : r.error, r.ok ? 'exito' : 'error');
      if (r.ok) cargar();
    });

    const efectivos = datos.permisos;
    cuerpo.replaceChildren(h('div', { class: 'pila-3' },
      h('div', { class: 'cabecera-pagina', style: { marginBottom: 0 } },
        h('div', { class: 'fila', style: { flexWrap: 'nowrap', gap: '16px' } }, h('span', { class: 'avatar', style: { width: '48px', height: '48px', fontSize: '18px' }, 'aria-hidden': 'true' }, iniciales(u.nombre)),
          h('div', {}, h('h2', {}, u.nombre), h('p', {}, u.email, ' · ', u.estado === 'activo' ? 'activo' : 'inactivo (no puede ingresar)'))),
        h('div', { class: 'fila' }, insignia(`${efectivos.length} permisos efectivos`, 'exito'), datos.overrides.length ? insignia(`${datos.overrides.length} personalizados`, 'info') : null)),
      datos.es_propietario ? aviso('Tiene el rol Propietario: acceso total que no se puede denegar, para que el sistema nunca quede sin administrador.', 'info') : null,
      tarjetaRoles,
      h('section', { class: 'tarjeta tarjeta-sin-relleno', 'aria-labelledby': 't-pers' },
        h('div', { class: 'tarjeta-cabecera' }, h('div', {}, h('h3', { id: 't-pers' }, 'Permisos por módulo'),
          h('p', { class: 'texto-2 texto-peq', style: { marginTop: '4px' } }, '«Heredado» usa el valor de sus roles. «Conceder» y «Denegar» lo personalizan solo para esta persona y prevalecen sobre el rol.')), restablecer),
        h('div', { class: 'leyenda', style: { padding: '0 24px 16px' } },
          h('span', {}, insignia('Personalizado', 'info', 'pencil'), 'prevalece sobre el rol'),
          h('span', {}, h('span', { class: 'muestra', style: { background: 'var(--color-advertencia-fondo)', border: '2px solid var(--color-advertencia)' } }), 'cambio sin guardar')),
        listaGrupos),
      barra,
      h('section', { class: 'tarjeta', 'aria-labelledby': 't-efec' },
        h('h3', { id: 't-efec', style: { marginBottom: '12px' } }, 'Permisos efectivos (guardados)'),
        efectivos.length ? h('div', { class: 'fila' }, efectivos.map((p) => insignia(p.descripcion, p.origen === 'personalizado' ? 'info' : 'exito', p.origen === 'personalizado' ? 'pencil' : 'check')))
          : vacio({ icono: 'lock', titulo: 'Sin permisos', texto: 'Esta persona puede ingresar pero no verá ningún módulo.' }))));
    datos.detalle.forEach(actualizarFila);
  }
  await cargar();
}
