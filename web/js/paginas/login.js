// Login de usuario (/login) y de administrador (/admin/login).
// Comparten componentes; el de administración es visualmente distinto y solo
// admite cuentas con permisos de administración.
import { h } from '../nucleo/dom.js';
import { icono } from '../nucleo/iconos.js';
import { url, navegar, parametros } from '../nucleo/router.js';
import { iniciarSesion, cerrarSesion, esAdmin } from '../nucleo/sesion.js';
import { MODULOS_ADMIN } from '../nucleo/modulos.js';
import { modulosVisibles, marca } from '../nucleo/shell.js';
import { aviso, boton, campo, botonIcono, conCarga } from '../nucleo/ui.js';

// Solo se acepta un destino interno (evita redirecciones abiertas).
function destinoSeguro(valor, porDefecto) {
  return valor && /^\/(app|admin)(\/|$|\?)/.test(valor) ? valor : porDefecto;
}

export default function login(raiz, { opciones }) {
  const admin = opciones.variante === 'admin';
  document.title = (admin ? 'Acceso administrador' : 'Iniciar sesión') + ' · La Gran Manzana';

  const estado = h('div', { 'aria-live': 'assertive', 'aria-atomic': 'true' });
  const email = h('input', { type: 'email', name: 'email', autocomplete: 'username', required: true, maxlength: '150', placeholder: 'nombre@tienda.com' });
  const clave = h('input', { type: 'password', name: 'password', autocomplete: 'current-password', required: true, maxlength: '72' });
  const verClave = botonIcono('eye', 'Mostrar contraseña', {
    onclick: () => {
      const visible = clave.type === 'text';
      clave.type = visible ? 'password' : 'text';
      verClave.setAttribute('aria-label', visible ? 'Mostrar contraseña' : 'Ocultar contraseña');
      verClave.setAttribute('aria-pressed', String(!visible));
    },
  });
  verClave.setAttribute('aria-pressed', 'false');
  const enviar = boton(admin ? 'Ingresar como administrador' : 'Iniciar sesión', { tipo: 'primario', submit: true, class: 'btn-bloque btn-grande' });

  const olvido = h('a', {
    href: '#', onclick: (e) => {
      e.preventDefault();
      estado.replaceChildren(aviso(admin
        ? 'Otro administrador puede asignarle una contraseña nueva desde Usuarios. Si es el único, use la opción -restablecer-clave en el servidor.'
        : 'Pídale a un administrador que le asigne una contraseña nueva desde la sección Usuarios.', 'info'));
    },
  }, '¿Olvidaste tu contraseña?');

  const form = h('form', { novalidate: true },
    campo('Correo', email),
    campo('Contraseña', h('div', { class: 'entrada-con-boton' }, clave, verClave)),
    h('div', { class: 'enlaces' }, olvido),
    enviar);
  // campo() asigna el id al contenedor de la contraseña; la etiqueta debe apuntar al input.
  const etiquetaClave = form.querySelectorAll('label')[1];
  clave.id = 'login-clave';
  etiquetaClave.setAttribute('for', 'login-clave');

  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    estado.replaceChildren();
    email.removeAttribute('aria-invalid');
    clave.removeAttribute('aria-invalid');
    if (!email.value.trim() || !email.checkValidity()) {
      email.setAttribute('aria-invalid', 'true');
      estado.replaceChildren(aviso('Escriba un correo válido.', 'error'));
      email.focus();
      return;
    }
    if (!clave.value) {
      clave.setAttribute('aria-invalid', 'true');
      estado.replaceChildren(aviso('Escriba su contraseña.', 'error'));
      clave.focus();
      return;
    }
    const r = await conCarga(enviar, iniciarSesion(email.value.trim(), clave.value));
    clave.value = '';
    if (!r.ok) {
      const mensaje = r.status === 401 ? 'Correo o contraseña incorrectos.' : r.error;
      estado.replaceChildren(aviso(mensaje, 'error'));
      clave.setAttribute('aria-invalid', 'true');
      clave.focus();
      return;
    }
    const siguiente = parametros().get('siguiente');
    if (admin) {
      if (!esAdmin()) {
        await cerrarSesion();
        estado.replaceChildren(aviso(h('span', {}, 'Esta cuenta no tiene permisos de administración. ', h('a', { href: url('/login'), 'data-enlace': '' }, 'Use el acceso de usuarios.')), 'error'));
        return;
      }
      navegar(destinoSeguro(siguiente, modulosVisibles(MODULOS_ADMIN)[0]?.ruta || '/app'), { reemplazar: true });
    } else {
      navegar(destinoSeguro(siguiente, '/app'), { reemplazar: true });
    }
  });

  const tarjeta = h('div', { class: 'tarjeta' },
    admin ? h('div', { class: 'franja-admin' }, icono('shield-check', { tam: 18 }), 'Área restringida · Administración') : null,
    h('div', { class: 'fila-entre' },
      h('h1', {}, admin ? 'Acceso de administración' : 'Iniciar sesión'),
      admin ? h('span', { class: 'insignia insignia-oscura' }, icono('shield', { tam: 12 }), 'Admin') : null),
    h('p', { class: 'subtitulo' }, admin
      ? 'Gestión de usuarios, roles y permisos. Solo para cuentas con permisos de administración.'
      : 'Ingrese con el correo y la contraseña que le asignó la administración.'),
    estado,
    form);

  raiz.append(h('main', { id: 'contenido', class: 'acceso' + (admin ? ' acceso-admin' : '') },
    h('div', { class: 'acceso-caja' },
      marca(admin ? 'marca-clara' : ''),
      tarjeta,
      h('p', { class: 'volver' }, admin
        ? h('a', { href: url('/login'), 'data-enlace': '' }, '← Volver al acceso de usuarios')
        : h('span', {}, h('a', { href: url('/'), 'data-enlace': '' }, '← Volver al inicio'), ' · ', h('a', { href: url('/admin/login'), 'data-enlace': '' }, 'Acceso administrador'))))));
  setTimeout(() => email.focus(), 0);
}
