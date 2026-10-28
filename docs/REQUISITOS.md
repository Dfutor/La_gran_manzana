# Cobertura del alcance entregado

| Requisito presente | Implementación / evidencia |
|---|---|
| Acceso y persistencia | Login bcrypt y roles sobre usuarios; escrituras MySQL con la conexión existente; formularios de operación. |
| Registro, actualización y consulta de productos/lotes | Catálogo/fichas/etiquetas; ingreso y corrección de lotes; stock derivado de cantidades vigentes. |
| Rotación FEFO | Simulación y venta transaccional por vencimiento/ID, excluyendo vencidos y agotados; prueba de venta concurrente. |
| Ventas y medios de pago | Encabezado/detalles, efectivo/Nequi/Daviplata, precios históricos, ofertas y rollback completo. |
| Alertas automáticas | Vigilancia cada minuto con avisos en logs del servicio; aviso global web cada 30 s y detalle de stock/vencimiento/acciones. |
| Pérdidas y acciones previas | Bajas por lote, usuario y costo; motivos y acciones del esquema; reporte agrupado por motivo/acción. |
| Análisis comercial | Ventas por día/pago/producto, productos sin movimiento, promedio diario, cobertura y sugerencia de reposición. |
| Consulta para empleados sin experiencia | Búsqueda por nombre/marca/código o necesidad; exclusión de etiquetas contraindicadas; ficha con uso, beneficios, ingredientes y contraindicaciones. |
| Información orientativa | Aviso de complemento en ficha y recomendaciones; contenido proviene del catálogo, sin diagnósticos ni generación de consejo médico. |
| Interfaz muy básica | HTML/CSS/JS sin frameworks, formularios y tablas; se conserva el aspecto mínimo. |
| Pruebas y despliegue | Unitarias, integración MySQL/concurrencia y flujo de navegador; instrucciones de pull, systemd, proxy y recuperación. |

Los 50 productos y demás cantidades de la documentación describen datos ficticios que ya estaban cargados; no se recrean ni reemplazan en producción. La aplicación también funciona con un catálogo vacío. Granel usa cantidades enteras de unidades de 100 g, como permite el esquema actual.

El chatbot de IA y la expansión a otros almacenes pertenecen al alcance futuro y no se implementan. Capacitar al personal, evaluar la practicidad con empleados reales y desplegar/verificar en el servidor son actividades de aceptación pendientes de ejecutar en la tienda. No hay cambios automáticos sobre producción. Las notificaciones externas (correo/SMS) no están configuradas; el canal automático actual es el log del servicio y el aviso web al mantener la sesión abierta.
