# Documentación técnica

La referencia operativa actual es [README.md](README.md), con servicios, permisos, configuración, primer acceso, pruebas y despliegue. [docs/API.md](docs/API.md) describe los contratos HTTP; [docs/REQUISITOS.md](docs/REQUISITOS.md) relaciona la implementación con el alcance entregado.

- `main.go`: rutas, servidor HTTP embebido, logs, documentación privada y apagado ordenado.
- `db.go`: `.env` y pool MySQL; conserva nombres y conexión de producción.
- `auth.go`: bcrypt, sesiones, permisos, administración de usuarios y comandos de primer acceso/recuperación.
- `handlers.go`, `validacion.go`: catálogo, fichas, etiquetas, consultas, FEFO y validación.
- `movimientos.go`: ventas/bajas transaccionales, ingreso/edición de lotes e importes exactos en centavos.
- `reportes.go`: historial paginado, analítica por periodo y vigilancia automática de alertas.
- `web/`: formularios/tablas HTML/CSS/JS sin frameworks; datos insertados como texto.
- `*_test.go`: reglas y seguridad; integración real opcional en base desechable.
- `deploy/`: ejemplos systemd y Apache para un servidor nuevo.

La base conserva sus diez tablas InnoDB; no hay migraciones, triggers ni cambios de esquema. Ventas y bajas bloquean productos/lotes y confirman todos sus cambios juntos. `subtotal` y `valor_perdido` los genera MySQL. Las asignaciones FEFO se devuelven al crear la venta; el esquema actual no tiene una tabla para conservar la relación venta-lote, por lo que el historial registra productos, cantidades y precios, no esa distribución.

Credenciales solo en `.env` o entorno, SQL parametrizado y dependencias limitadas al driver MySQL y bcrypt. `docs/esquema.sql` es una copia de referencia con DROP TABLE: usar únicamente para crear bases de prueba vacías. No modificar producción, Apache/PHP ni datos existentes como parte de una prueba. El ingreso de usuarios, productos, ventas y demás operaciones autorizadas sí persiste en MySQL.
