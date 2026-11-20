// Módulos de la aplicación y el permiso que exige cada uno (basta con uno de la lista).
// Un módulo sin permiso no se muestra en la navegación ni en el panel.

const CRUD = (r) => [`${r}.crear`, `${r}.editar`, `${r}.eliminar`];

export const MODULOS = [
  { id: 'inicio', ruta: '/app', titulo: 'Inicio', icono: 'layout-dashboard', permisos: [] },
  { id: 'productos', ruta: '/app/productos', titulo: 'Productos', icono: 'package', permisos: ['productos.ver'], descripcion: 'Buscar productos, fichas y lotes FEFO' },
  { id: 'consulta', ruta: '/app/consulta', titulo: 'Consulta por necesidad', icono: 'heart-pulse', permisos: ['recomendaciones.consultar'], descripcion: 'Qué ofrecer según lo que el cliente necesita' },
  { id: 'ventas', ruta: '/app/ventas', titulo: 'Ventas', icono: 'shopping-cart', permisos: ['ventas.crear', 'ventas.ver'], descripcion: 'Registrar ventas y consultar el historial' },
  { id: 'inventario', ruta: '/app/inventario', titulo: 'Lotes y bajas', icono: 'boxes', permisos: ['lotes.ver', 'lotes.crear', 'lotes.editar', 'bajas.ver', 'bajas.crear'], descripcion: 'Ingresos de lotes, correcciones y bajas' },
  { id: 'alertas', ruta: '/app/alertas', titulo: 'Alertas', icono: 'bell', permisos: ['alertas.ver'], descripcion: 'Vencimientos y stock bajo' },
  { id: 'catalogo', ruta: '/app/catalogo', titulo: 'Catálogo', icono: 'tags', permisos: [...CRUD('productos'), ...CRUD('categorias'), ...CRUD('proveedores'), ...CRUD('etiquetas')], descripcion: 'Productos, categorías, proveedores y etiquetas' },
  { id: 'reportes', ruta: '/app/reportes', titulo: 'Reportes', icono: 'chart-column', permisos: ['reportes.ventas', 'reportes.perdidas', 'reportes.rotacion'], descripcion: 'Ventas, pérdidas y rotación' },
];

export const MODULOS_ADMIN = [
  { id: 'usuarios', ruta: '/admin/usuarios', titulo: 'Usuarios', icono: 'users', permisos: ['usuarios.ver'], descripcion: 'Cuentas, estado y permisos efectivos' },
  { id: 'roles', ruta: '/admin/roles', titulo: 'Roles y permisos', icono: 'shield-check', permisos: ['roles.ver'], descripcion: 'Matriz de permisos por rol' },
  { id: 'auditoria', ruta: '/admin/auditoria', titulo: 'Auditoría', icono: 'scroll-text', permisos: ['auditoria.ver'], descripcion: 'Quién cambió qué acceso y cuándo' },
  { id: 'probador', ruta: '/admin/probador', titulo: 'Probador de API', icono: 'terminal', permisos: ['usuarios.ver', 'roles.ver', 'auditoria.ver'], descripcion: 'Enviar peticiones al API' },
];
