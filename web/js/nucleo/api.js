// Cliente del API. Nunca lanza excepción: devuelve {ok, status, data, error}.
// Emite eventos globales para que la sesión reaccione a 401 (sesión vencida)
// y a 403 (los permisos pudieron cambiar mientras la pantalla estaba abierta).

export const eventosApi = new EventTarget();

export async function api(ruta, { metodo = 'GET', cuerpo } = {}) {
  const opciones = { method: metodo, headers: { Accept: 'application/json' }, credentials: 'same-origin' };
  if (cuerpo !== undefined) {
    opciones.headers['Content-Type'] = 'application/json';
    opciones.body = typeof cuerpo === 'string' ? cuerpo : JSON.stringify(cuerpo);
  }
  let resp;
  try {
    resp = await fetch(ruta, opciones);
  } catch {
    return { ok: false, status: 0, data: null, error: 'No hay conexión con el servidor. Revise su red e intente de nuevo.' };
  }
  const crudo = await resp.text();
  let data = null;
  try { data = crudo ? JSON.parse(crudo) : null; } catch { data = null; }
  const error = resp.ok ? '' : (data && data.error) || `Error ${resp.status}`;
  if (resp.status === 401 && ruta !== '/api/login') eventosApi.dispatchEvent(new Event('sesion-vencida'));
  if (resp.status === 403 && ruta !== '/api/sesion') eventosApi.dispatchEvent(new Event('sin-permiso'));
  return { ok: resp.ok, status: resp.status, data, error, crudo };
}

export const get = (ruta) => api(ruta);
export const post = (ruta, cuerpo) => api(ruta, { metodo: 'POST', cuerpo });
export const put = (ruta, cuerpo) => api(ruta, { metodo: 'PUT', cuerpo });
export const del = (ruta) => api(ruta, { metodo: 'DELETE' });
