// Matriz de roles y permisos: filas = permisos (agrupados por módulo), columnas = roles.
// Cada celda es un interruptor (role="switch"). Los cambios quedan pendientes hasta
// guardar; se muestra un resumen de lo que cambia y a cuántos usuarios afecta.
import { h } from '../../nucleo/dom.js';
import { icono } from '../../nucleo/iconos.js';
import { get, post, put, del } from '../../nucleo/api.js';
import { puede, usuarioActual } from '../../nucleo/sesion.js';
import { insignia, aviso, esqueletoTabla, errorCarga, boton, botonIcono, notificar, confirmar, modalFormulario, campo, selector, conCarga, interruptor, interruptorConEtiqueta } from '../../nucleo/ui.js';

export default async function roles(contenedor) {
  const caja = h('div', {}, esqueletoTabla(8));
  const gestiona = puede('roles.gestionar');
  contenedor.append(
    h('div', { class: 'cabecera-pagina' },
      h('div', {}, h('h2', {}, 'Roles y permisos'), h('p', {}, 'Active o desactive permisos por rol. Los usuarios de cada rol reciben los cambios en su siguiente acción.')),
      gestiona ? boton('Nuevo rol', { tipo: 'primario', icono: 'plus', onclick: () => nuevoRol() }) : null),
    caja);

  let catalogo = [];
  let lista = [];
  const yo = usuarioActual();
  const propios = () => new Set(usuarioActual().permisos);

  async function cargar() {
    caja.replaceChildren(esqueletoTabla(8));
    const [rp, rr] = await Promise.all([get('/api/permisos'), get('/api/roles')]);
    if (!rp.ok || !rr.ok) { caja.replaceChildren(errorCarga(rp.ok ? rr : rp, cargar)); return; }
    catalogo = rp.data;
    lista = rr.data;
    pintar();
  }

  // Por qué no se puede editar un rol (o '' si se puede).
  function bloqueo(rol) {
    if (!gestiona) return 'Necesita el permiso roles.gestionar';
    if (rol.es_sistema) return 'Rol del sistema: siempre tiene todos los permisos';
    if (yo.roles.some((r) => r.id === rol.id)) return 'Es un rol suyo: otro administrador debe editarlo';
    const faltan = rol.permisos.filter((p) => !propios().has(p));
    if (faltan.length) return `Incluye permisos que usted no tiene (${faltan.length})`;
    return '';
  }

  function pintar() {
    const mios = propios();
    const original = new Map(lista.map((r) => [r.id, new Set(r.permisos)]));
    const pendiente = new Map(lista.map((r) => [r.id, new Set(r.permisos)]));
    const editables = lista.filter((r) => !bloqueo(r));
    const filasSel = new Set();
    const colsSel = new Set();
    const celdas = new Map(); // `${rol}|${permiso}` → {td, sw}
    const filasTr = new Map();
    const columnasTh = new Map();

    const editableCelda = (rol, permiso) => !bloqueo(rol) && mios.has(permiso);

    // --- Barra de cambios pendientes ---
    const resumenCambios = h('span', { 'aria-live': 'polite' });
    const guardar = boton('Revisar y guardar', { tipo: 'primario', icono: 'check' });
    const descartar = boton('Descartar', { icono: 'undo-2' });
    const barraCambios = h('div', { class: 'barra-cambios', hidden: true, role: 'region', 'aria-label': 'Cambios pendientes' }, resumenCambios, h('div', { class: 'fila' }, descartar, guardar));
    function diferencias() {
      return lista.map((r) => {
        const a = original.get(r.id), b = pendiente.get(r.id);
        return { rol: r, agregados: [...b].filter((p) => !a.has(p)), quitados: [...a].filter((p) => !b.has(p)) };
      }).filter((d) => d.agregados.length || d.quitados.length);
    }
    function actualizarBarra() {
      const difs = diferencias();
      const total = difs.reduce((s, d) => s + d.agregados.length + d.quitados.length, 0);
      barraCambios.hidden = !total;
      resumenCambios.textContent = `${total} permiso${total === 1 ? '' : 's'} cambian en ${difs.length} rol${difs.length === 1 ? '' : 'es'}`;
    }
    function fijarCelda(rol, permiso, valor) {
      const set = pendiente.get(rol.id);
      if (valor) set.add(permiso); else set.delete(permiso);
      const c = celdas.get(`${rol.id}|${permiso}`);
      if (c?.sw) c.sw.setAttribute('aria-checked', String(valor));
      c?.td.classList.toggle('pendiente', original.get(rol.id).has(permiso) !== valor);
    }

    // --- Selección masiva ---
    const resumenSel = h('span', {});
    const barraSel = h('div', { class: 'barra-seleccion', hidden: true, role: 'region', 'aria-label': 'Acciones sobre la selección' }, icono('filter', { tam: 16 }), resumenSel,
      boton('Activar en seleccionados', { peq: true, tipo: 'primario', icono: 'check', onclick: () => aplicar(true) }),
      boton('Desactivar en seleccionados', { peq: true, icono: 'minus', onclick: () => aplicar(false) }),
      boton('Limpiar selección', { peq: true, tipo: 'fantasma', onclick: () => limpiarSeleccion() }));
    const objetivo = () => ({
      permisos: filasSel.size ? [...filasSel] : catalogo.map((p) => p.nombre),
      roles: colsSel.size ? editables.filter((r) => colsSel.has(r.id)) : editables,
    });
    function actualizarSeleccion() {
      for (const [p, tr] of filasTr) { tr.classList.toggle('fila-seleccionada', filasSel.has(p)); tr.querySelector('input[type=checkbox]').checked = filasSel.has(p); }
      for (const [id, celdasCol] of columnasTh) { celdasCol.forEach((el) => el.classList.toggle('col-seleccionada', colsSel.has(id))); }
      for (const g of grupoChecks) {
        const nombres = g.permisos.map((p) => p.nombre);
        const n = nombres.filter((x) => filasSel.has(x)).length;
        g.check.checked = n === nombres.length;
        g.check.indeterminate = n > 0 && n < nombres.length;
      }
      const hay = filasSel.size || colsSel.size;
      barraSel.hidden = !hay;
      const { permisos, roles: rs } = objetivo();
      resumenSel.textContent = `${permisos.length} permiso${permisos.length === 1 ? '' : 's'} × ${rs.length} rol${rs.length === 1 ? '' : 'es'} seleccionados`;
    }
    function aplicar(valor) {
      const { permisos, roles: rs } = objetivo();
      let n = 0;
      for (const r of rs) for (const p of permisos) {
        if (editableCelda(r, p) && pendiente.get(r.id).has(p) !== valor) { fijarCelda(r, p, valor); n++; }
      }
      actualizarBarra();
      notificar(n ? `${n} celda${n === 1 ? '' : 's'} ${valor ? 'activadas' : 'desactivadas'} (sin guardar).` : 'No hubo celdas editables que cambiar.', 'info', 3000);
    }
    function limpiarSeleccion() { filasSel.clear(); colsSel.clear(); actualizarSeleccion(); }

    // --- Cabecera: un rol por columna ---
    const cabecera = h('tr', {}, h('th', { class: 'col-permiso', scope: 'col' }, h('span', { class: 'texto-mini' }, 'Permiso')));
    for (const r of lista) {
      const motivo = bloqueo(r);
      const th = h('th', { scope: 'col', title: motivo || '' });
      const check = h('input', { type: 'checkbox', disabled: !!motivo, 'aria-label': `Seleccionar columna ${r.nombre}` });
      check.addEventListener('change', () => { if (check.checked) colsSel.add(r.id); else colsSel.delete(r.id); actualizarSeleccion(); });
      th.append(h('div', { class: 'rol-cabecera' },
        gestiona ? h('label', { class: 'check texto-mini' }, check, 'Seleccionar') : null,
        h('span', { class: 'nombre-rol' }, r.nombre),
        h('div', { class: 'fila', style: { justifyContent: 'center' } },
          r.es_sistema ? insignia('Sistema', 'oscura', 'lock') : null,
          !r.activo ? insignia('Inactivo', 'error') : null,
          insignia(`${r.cantidad_usuarios} usuario${r.cantidad_usuarios === 1 ? '' : 's'}`)),
        motivo && !r.es_sistema ? h('span', { class: 'texto-mini', style: { color: 'var(--color-texto-2)', fontWeight: 500, textTransform: 'none', letterSpacing: 0 } }, icono('lock', { tam: 12 }), ' ', motivo) : null,
        gestiona && !r.es_sistema ? h('div', { class: 'fila', style: { justifyContent: 'center' } },
          botonIcono('pencil', `Editar datos del rol ${r.nombre}`, { disabled: !!motivo, onclick: () => editarRol(r, diferencias().length) }),
          botonIcono('copy', `Duplicar el rol ${r.nombre}`, { onclick: () => nuevoRol(r) }),
          botonIcono('trash-2', `Eliminar el rol ${r.nombre}`, { disabled: !!motivo, onclick: () => eliminarRol(r) })) : null));
      cabecera.append(th);
      columnasTh.set(r.id, [th]);
    }

    // --- Cuerpo: módulos y permisos ---
    const grupoChecks = [];
    const cuerpo = h('tbody', {});
    const modulos = [];
    for (const p of catalogo) {
      let m = modulos.find((x) => x.nombre === p.modulo);
      if (!m) modulos.push(m = { nombre: p.modulo, permisos: [] });
      m.permisos.push(p);
    }
    for (const m of modulos) {
      const checkGrupo = h('input', { type: 'checkbox', 'aria-label': `Seleccionar todos los permisos de ${m.nombre}` });
      checkGrupo.addEventListener('change', () => { m.permisos.forEach((p) => (checkGrupo.checked ? filasSel.add(p.nombre) : filasSel.delete(p.nombre))); actualizarSeleccion(); });
      grupoChecks.push({ check: checkGrupo, permisos: m.permisos });
      cuerpo.append(h('tr', { class: 'grupo' }, h('th', { scope: 'rowgroup', colspan: String(lista.length + 1) }, h('label', { class: 'check' }, gestiona ? checkGrupo : null, m.nombre))));
      for (const p of m.permisos) {
        const checkFila = h('input', { type: 'checkbox', 'aria-label': `Seleccionar fila ${p.descripcion}` });
        checkFila.addEventListener('change', () => { if (checkFila.checked) filasSel.add(p.nombre); else filasSel.delete(p.nombre); actualizarSeleccion(); });
        const tr = h('tr', {}, h('th', { class: 'col-permiso', scope: 'row' },
          h('div', { class: 'fila', style: { flexWrap: 'nowrap', alignItems: 'flex-start' } }, gestiona ? checkFila : null,
            h('div', {}, h('span', { class: 'permiso-desc' }, p.descripcion), p.critico ? h('span', { class: 'texto-mini', style: { color: 'var(--color-error)', marginLeft: '6px' } }, 'crítico') : null,
              h('code', { class: 'permiso-codigo' }, p.nombre), !mios.has(p.nombre) ? h('span', { class: 'permiso-codigo' }, icono('lock', { tam: 11 }), ' usted no lo tiene') : null))));
        for (const r of lista) {
          const td = h('td', { class: 'celda' });
          const activo = pendiente.get(r.id).has(p.nombre);
          let sw = null;
          if (r.es_sistema) td.append(h('span', { class: 'celda-bloqueada', title: 'Rol del sistema' }, icono('lock', { tam: 14 }), h('span', { class: 'solo-lectores' }, 'Activo, bloqueado'), icono('check', { tam: 16 })));
          else {
            sw = interruptor({ activo, etiqueta: `${p.descripcion} para ${r.nombre}`, deshabilitado: !editableCelda(r, p.nombre), alCambiar: (v) => { fijarCelda(r, p.nombre, v); actualizarBarra(); } });
            td.append(sw);
          }
          celdas.set(`${r.id}|${p.nombre}`, { td, sw });
          columnasTh.get(r.id).push(td);
          tr.append(td);
        }
        filasTr.set(p.nombre, tr);
        cuerpo.append(tr);
      }
    }

    descartar.addEventListener('click', () => {
      for (const r of lista) for (const p of catalogo) fijarCelda(r, p.nombre, original.get(r.id).has(p.nombre));
      actualizarBarra();
    });
    guardar.addEventListener('click', async () => {
      const difs = diferencias();
      const afectados = difs.reduce((s, d) => s + d.rol.cantidad_usuarios, 0);
      const ok = await confirmar({
        titulo: 'Confirmar cambios de permisos',
        mensaje: h('div', { class: 'pila' },
          h('p', {}, `Se actualizarán ${difs.length} rol${difs.length === 1 ? '' : 'es'}; ${afectados} usuario${afectados === 1 ? '' : 's'} recibirán los cambios en su siguiente acción.`),
          h('ul', { class: 'lista-limpia pila-1' }, difs.map((d) => h('li', { class: 'tarjeta', style: { padding: '12px 16px' } },
            h('div', { class: 'fila-entre' }, h('strong', {}, d.rol.nombre), h('span', { class: 'fila' }, d.agregados.length ? insignia(`+${d.agregados.length}`, 'exito') : null, d.quitados.length ? insignia(`−${d.quitados.length}`, 'error') : null)),
            d.agregados.length ? h('p', { class: 'texto-mini texto-2' }, 'Agrega: ', d.agregados.join(', ')) : null,
            d.quitados.length ? h('p', { class: 'texto-mini texto-2' }, 'Quita: ', d.quitados.join(', ')) : null)))),
        confirmar: 'Guardar cambios',
      });
      if (!ok) return;
      await conCarga(guardar, (async () => {
        const errores = [];
        for (const d of difs) {
          const r = await put('/api/roles/' + d.rol.id, { nombre: d.rol.nombre, descripcion: d.rol.descripcion, activo: d.rol.activo, permisos: [...pendiente.get(d.rol.id)] });
          if (!r.ok) errores.push(`${d.rol.nombre}: ${r.error}`);
        }
        if (errores.length) notificar('Algunos roles no se guardaron. ' + errores.join(' · '), 'error', 9000);
        else notificar(`Permisos guardados. ${afectados} usuario${afectados === 1 ? '' : 's'} ya tienen sus permisos efectivos actualizados.`, 'exito', 6000);
      })());
      cargar();
    });

    caja.replaceChildren(h('div', { class: 'pila' },
      lista.length < 2 ? aviso('Solo existe el rol del sistema. Cree un rol para empezar a asignar permisos por función.', 'info') : null,
      h('div', { class: 'leyenda' },
        h('span', {}, h('span', { class: 'muestra', style: { background: 'var(--color-primario)' }, 'aria-hidden': 'true' }), 'Permiso activo'),
        h('span', {}, h('span', { class: 'muestra', style: { background: 'var(--color-advertencia-fondo)', border: '2px solid var(--color-advertencia)' } }), 'Cambio sin guardar'),
        h('span', {}, icono('lock', { tam: 14 }), 'No editable (rol del sistema, rol propio o permiso que usted no tiene)')),
      barraSel,
      h('div', { class: 'matriz-contenedor' }, h('table', { class: 'matriz', 'aria-label': 'Matriz de permisos por rol' }, h('thead', {}, cabecera), cuerpo)),
      barraCambios));
  }

  function nuevoRol(base) {
    const mios = propios();
    modalFormulario({
      titulo: base ? `Duplicar «${base.nombre}»` : 'Nuevo rol',
      descripcion: 'Después podrá ajustar sus permisos en la matriz.',
      campos: h('div', { class: 'pila' },
        campo('Nombre', h('input', { name: 'nombre', required: true, maxlength: '60', value: base ? `${base.nombre} (copia)`.slice(0, 60) : '' }), { ayuda: 'Por ejemplo: Gerente, Operador, Consulta' }),
        campo('Descripción', h('input', { name: 'descripcion', maxlength: '255', value: base?.descripcion || '' })),
        campo('Permisos iniciales', selector([{ valor: '', texto: 'Ninguno (empezar vacío)' }, ...lista.map((r) => ({ valor: r.id, texto: `Copiar de ${r.nombre}`, seleccionado: r.id === base?.id }))], { name: 'base' }),
          { ayuda: 'Solo se copian los permisos que usted tiene.' })),
      textoEnviar: 'Crear rol',
      enviar: async (f) => {
        const origen = lista.find((r) => String(r.id) === f.elements.base.value);
        const permisos = origen ? origen.permisos.filter((p) => mios.has(p)) : [];
        const r = await post('/api/roles', { nombre: f.elements.nombre.value, descripcion: f.elements.descripcion.value, permisos });
        if (r.ok) {
          const omitidos = origen ? origen.permisos.length - permisos.length : 0;
          notificar(`Rol «${r.data.nombre}» creado${omitidos ? `; se omitieron ${omitidos} permisos que usted no tiene` : ''}.`, 'exito');
          cargar();
        }
        return r;
      },
    });
  }

  function editarRol(rol, hayPendientes) {
    if (hayPendientes) { notificar('Guarde o descarte los cambios de la matriz antes de editar los datos de un rol.', 'error'); return; }
    let activo = rol.activo;
    const { elemento } = interruptorConEtiqueta('Rol activo (un rol inactivo no otorga permisos)', { activo, alCambiar: (v) => { activo = v; } });
    modalFormulario({
      titulo: `Editar rol «${rol.nombre}»`,
      campos: h('div', { class: 'pila' },
        campo('Nombre', h('input', { name: 'nombre', required: true, maxlength: '60', value: rol.nombre })),
        campo('Descripción', h('input', { name: 'descripcion', maxlength: '255', value: rol.descripcion })),
        elemento,
        rol.cantidad_usuarios ? aviso(`Afecta a ${rol.cantidad_usuarios} usuario${rol.cantidad_usuarios === 1 ? '' : 's'}.`, 'info') : null),
      enviar: async (f) => {
        const r = await put('/api/roles/' + rol.id, { nombre: f.elements.nombre.value, descripcion: f.elements.descripcion.value, activo, permisos: rol.permisos });
        if (r.ok) { notificar('Rol actualizado.', 'exito'); cargar(); }
        return r;
      },
    });
  }

  async function eliminarRol(rol) {
    if (rol.cantidad_usuarios) { notificar(`«${rol.nombre}» tiene ${rol.cantidad_usuarios} usuario(s): quíteselo o desactívelo antes de eliminarlo.`, 'error', 7000); return; }
    const ok = await confirmar({ titulo: 'Eliminar rol', mensaje: `¿Eliminar el rol «${rol.nombre}»? Esta acción no se puede deshacer.`, confirmar: 'Eliminar', peligro: true });
    if (!ok) return;
    const r = await del('/api/roles/' + rol.id);
    notificar(r.ok ? `Rol «${rol.nombre}» eliminado.` : r.error, r.ok ? 'exito' : 'error');
    if (r.ok) cargar();
  }

  await cargar();
}
