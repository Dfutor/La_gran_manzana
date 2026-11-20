# Rediseño de la interfaz

Frontend en HTML, CSS y JavaScript (módulos ES) sin frameworks ni paso de compilación. Todo queda embebido en el binario de Go: el despliegue no cambia y no se usa ningún CDN. La fuente Inter (OFL) y los íconos Lucide (ISC) están incluidos en el repositorio.

## Rutas

Todas viven bajo `/granmanzana`. El servidor responde `index.html` a cualquier ruta sin extensión y el enrutador del navegador (History API) resuelve la página. Los recursos inexistentes siguen dando 404.

| Ruta | Acceso | Página |
|---|---|---|
| `/` | público | Landing |
| `/login` | público | Login de usuario |
| `/admin/login` | público | Login de administrador: rechaza cuentas sin permisos de administración |
| `/app` | sesión | Panel: recomendaciones, métricas y accesos rápidos |
| `/app/productos`, `/consulta`, `/ventas`, `/inventario`, `/alertas`, `/catalogo`, `/reportes` | sesión + permiso del módulo | Módulos de operación |
| `/admin/usuarios`, `/admin/usuarios/{id}`, `/admin/roles`, `/admin/auditoria`, `/admin/probador` | sesión + permiso de administración | Panel de administración |

`/admin/login` no es `/admin/login` en la raíz del servidor porque Apache solo envía a la aplicación `/granmanzana/` y `/api/`.

Si alguien entra sin sesión a una ruta protegida, se le envía al login correspondiente con `?siguiente=`; solo se aceptan destinos internos. Un módulo sin permiso no aparece en la navegación, el panel ni los accesos rápidos, y si se escribe su URL se muestra "No tiene acceso". El servidor valida cada permiso de nuevo.

## Estructura

```
web/
  index.html                 shell HTML (<base href="/granmanzana/">)
  fonts/                     Inter variable (latin)
  css/tokens.css             colores, tipografía, espaciado, radios, sombras
  css/base.css               reset, tipografía, foco visible, utilidades
  css/componentes.css        botones, campos, tarjetas, insignias, tablas, interruptor,
                             control segmentado, pestañas, avisos, toasts, skeleton,
                             estados vacíos, diálogos, paginación
  css/paginas.css            landing, logins, shell, panel, matriz, permisos, responsive
  js/main.js                 rutas, guardas de acceso, renderizado
  js/nucleo/                 dom (h(), formato), api, sesion (puede()), router, shell,
                             modulos (permiso de cada módulo), ui (componentes), iconos
  js/paginas/                landing, login, dashboard, productos, consulta, ventas,
                             inventario, alertas, catalogo, reportes
  js/paginas/admin/          usuarios, usuario-permisos, roles (matriz), auditoria, probador
```

Para agregar un módulo: se crea su página en `js/paginas/`, se registra en `nucleo/modulos.js` con sus permisos y se agrega su ruta en `main.js`. La navegación y el panel lo muestran solos según los permisos.

## Sistema de diseño

Los tokens están en `web/css/tokens.css`:

- **Colores:** primario `#2E7D32`, oscuro `#1B5E20` (navegación lateral, login de administrador), claro `#66BB6A` (solo acentos), fondo `#F1F8F4`, superficie `#FFFFFF`, texto `#1F2A22` / `#5B6B5F`, y los estados éxito, advertencia, error e info.
- **Contraste:** todo texto cumple WCAG AA (≥ 4.5:1); los pares verificados figuran en el archivo. `#66BB6A` y `#F9A825` no alcanzan 4.5:1 con texto, así que se usan solo como fondo o acento; el texto de advertencia usa `#7A5200`.
- **Tipografía:** Inter con respaldo `system-ui`. Escala: 28/22/18 para títulos, 15/14 para el cuerpo y 13/12 para etiquetas.
- **Espaciado:** múltiplos de 8. **Radios:** 8, 12 y 16. **Sombras:** tres niveles suaves.

En accesibilidad:
- Hay un enlace para saltar al contenido y el foco es visible en todo.
- Todos los campos tienen etiqueta y los errores se anuncian por `aria-live` o `role="alert"`.
- Los interruptores usan `role="switch"` con `aria-checked`, y las pestañas siguen el patrón `tablist` con flechas del teclado.
- Los diálogos usan `<dialog>` nativo, que gestiona el foco y se cierra con Esc.
- El menú móvil expone `aria-expanded` y también se cierra con Esc.
- Se respeta `prefers-reduced-motion`.

El diseño es responsive en tres tamaños:
- **Escritorio:** navegación lateral fija.
- **≤ 960 px:** la navegación pasa a un cajón con botón de menú.
- **≤ 640 px:** todo va en una columna. La matriz de permisos se desplaza dentro de su caja, sin desplazamiento horizontal de la página.

## Decisiones

- **Datos reales, sin mocks.** El RBAC ya existe en el backend, así que la interfaz usa el API real. Para probar hay datos de ejemplo en `docs/fixture_pruebas.sql`.
- **Roles base.** Se conservan Propietario (del sistema, acceso total, no editable) y Empleado. Administrador, Gerente, Operador o Consulta se crean desde la matriz, porque los roles son dinámicos.
- **Matriz.** Las filas son los permisos reales agrupados por módulo, no un "ver/crear/editar/eliminar/aprobar" genérico: no todos los módulos tienen esas acciones, y "aprobar" no existe en el negocio. Las celdas que el administrador no puede cambiar se muestran bloqueadas con el motivo: rol del sistema, rol propio o permiso que no tiene.
- **Selección masiva.** Hay casillas por fila, por módulo y por columna. "Activar/Desactivar en seleccionados" aplica sobre la intersección; si solo hay filas seleccionadas, aplica sobre todos los roles editables. Antes de guardar se muestra un resumen con los permisos agregados y quitados por rol y cuántos usuarios se ven afectados.
- **Permisos por usuario.** Cada permiso tiene tres estados: Heredado (lo que dan sus roles), Conceder o Denegar. La prioridad es override del usuario > rol > denegado. A quien tiene el rol Propietario no se le puede denegar nada, para que el sistema nunca quede sin administrador.
- **Recomendaciones del panel.** Salen de las alertas de vencimiento y stock bajo (`GET /api/sugerencias`). "Aceptar" registra la decisión y lleva a la acción: registrar baja, ingresar lote o abrir la ficha. "Descartar" las oculta para todo el equipo. Si la situación cambia (el lote pasa de "liquidar" a "vencido", o el stock vuelve a bajar), la recomendación reaparece.
- **Crear cuenta.** El formulario incluye la contraseña inicial además de nombre, correo y rol, porque no hay envío de correos. Tiene un botón para generar una contraseña aleatoria.
- **"¿Olvidaste tu contraseña?"** No simula un flujo que no existe: explica que un administrador asigna una contraseña nueva desde Usuarios.

## Pendiente

- **Recuperación de contraseña por correo:** requiere configurar un servicio de correo; hoy la restablece un administrador.
- **Modo oscuro:** opcional en el pedido. Los tokens están listos para agregarlo, pero falta definir y verificar los contrastes de la paleta oscura.
- **Deshacer un "Descartar":** el API registra la decisión pero no tiene una ruta para revertirla.
