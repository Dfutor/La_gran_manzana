# Pruebas

`go test -race ./...` verifica reglas FEFO, importes exactos, validación, rutas, permisos, bloqueo de accesos sin sesión, rechazo de Origin ajeno, límite de login y documentación privada. No conecta MySQL sin `GM_INTEGRACION=1`.

La integración requiere MySQL 8 y una **base desechable exclusiva**, con nombre que empiece por `granmanzana_test`. Rechaza otro nombre para evitar ejecutar pruebas sobre producción. Usa credenciales distintas de producción con SELECT/INSERT/UPDATE/DELETE sobre esa base. En una base vacía, un administrador puede cargar:

```bash
# -p solicita la clave; ajustar host, puerto y usuario del MySQL de pruebas.
mysql -h 127.0.0.1 -u root -p -e 'CREATE DATABASE granmanzana_test CHARACTER SET utf8mb4;'
mysql -h 127.0.0.1 -u root -p granmanzana_test < docs/esquema.sql
mysql -h 127.0.0.1 -u root -p granmanzana_test < docs/fixture_pruebas.sql
mysql -h 127.0.0.1 -u root -p granmanzana_test < docs/migracion_rbac.sql
# Definir DB_USER, DB_PASS, DB_HOST y DB_NAME del entorno de pruebas.
GM_INTEGRACION=1 DB_NAME=granmanzana_test go test -race -count=1 ./...
```

Las pruebas crean sus propios usuarios con bcrypt e ingresan por el login real; el primero recibe el rol Propietario. Verifican stock contra SQL, recomendaciones, alertas, CRUD, FEFO entre lotes, exclusión de vencidos, rollback al fallar el segundo producto, concurrencia de dos ventas, bajas sin stock, precio histórico, lectura de reportes y revocación al desactivar/cambiar la clave. En RBAC verifican: usuario nuevo sin acceso; rol personalizado aplicado en la siguiente petición; editar, desactivar y quitar un rol cambia el acceso sin volver a ingresar; alcance de ventas propias; permisos efectivos con su origen; un administrador delegado no puede otorgar permisos que no tiene, editar su propio rol ni el del sistema, cambiarse sus roles, asignar Propietario ni degradar al propietario; no se elimina un rol asignado; nombres duplicados; sincronización de `usuarios.rol`; y registro en auditoría. Al terminar eliminan los registros de prueba; nunca ejecutar con datos reales, incluso si la base tiene el prefijo permitido.

Validación manual mínima en ese mismo entorno:

1. Crear un propietario con `-crear-propietario`; ingresar, crear un usuario y asignarle el rol Empleado en **Roles y permisos**.
2. Crear producto, etiquetas y dos lotes con vencimientos distintos; comprobar ficha y búsqueda.
3. Registrar una venta que consuma ambos lotes; verificar total y cantidades restantes.
4. Intentar vender más que el stock: 409 sin cambios parciales; dar de baja una unidad y consultar su pérdida.
5. Abrir reportes, revisar reposición y comprobar el aviso de alertas desde otra pestaña.
6. Ingresar como empleado: puede operar ventas/lotes/bajas, no administrar catálogo/usuarios ni reportes.
7. Crear un rol «Cajero» solo con productos y ventas, asignarlo a otro usuario y comprobar que solo ve esas pestañas y sus propias ventas. Quitarle el rol desde la sesión del propietario: su siguiente acción recibe 403.
8. Revisar la auditoría: aparecen la creación del rol y las asignaciones con su autor.
9. Cerrar sesión y reiniciar: exige ingresar de nuevo. Desactivar al empleado revoca su acceso.

La aceptación con personal real debe registrar fecha, participante, escenario, resultado y observaciones sobre uso y claridad de las fichas. Esta actividad y la verificación del servidor de producción no se reemplazan con datos ficticios.
