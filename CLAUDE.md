# La Gran Manzana — API en Go + frontend de prueba

## Contexto
Proyecto de la materia Calidad de Software (Universidad Piloto de Colombia, seccional Alto Magdalena):
inventario por lotes con FEFO y consulta/recomendación de productos para la tienda naturista
«La Gran Manzana» (Melgar, Tolima).

- Servidor: VM de Azure `maquinola`, Ubuntu 24.04, usuario `makina`, 2 vCPU, ~4 GB de RAM + 2 GB de swap (`/swapfile`), zona horaria America/Bogota.
- IP pública: 158.158.17.188. Frontend: http://158.158.17.188/granmanzana/ · API: http://158.158.17.188/api/
- Apache 2.4 + PHP 8.3 sirven los ejemplos del profesor en /var/www/html; Apache hace de proxy inverso hacia el API.
- Go 1.27 en /usr/local/go (instalado desde go.dev; no se usó snap).

## Estructura
- `main.go`: configuración, rutas, middlewares (log, CORS, recuperación de panics), servidor y apagado ordenado.
- `db.go`: lectura de `.env` y pool de MySQL con `mysql.NewConfig()` (ParseTime, 10 conexiones).
- `handlers.go`: endpoints, modelos y reglas de negocio puras (`clasificarVencimiento`, `validarNombreCategoria`, `escaparLike`).
- `handlers_test.go`: pruebas unitarias en tabla (reglas puras, validaciones, rutas, autenticación de docs); no requieren base de datos.
- `integracion_test.go`: pruebas contra MySQL real; solo corren con `GM_INTEGRACION=1`. Crean y borran sus propias categorías.
- `web/`: frontend **deliberadamente básico** (HTML/CSS/JS puros) embebido con `go:embed` y servido en `/granmanzana/`. El diseño visual no es parte de esta entrega.
- `docs_web/`: documentación privada para los expositores (tutorial de Postman + colección), servida en `/granmanzana/docs/` con HTTP Basic (DOCS_USER/DOCS_PASS en `.env`).
- `docs/API.md`: documentación para el profesor. `docs/esquema.sql`: esquema sin datos, que es la **fuente de verdad** de columnas, tipos y ENUM.

## Comandos
```bash
# Probar
gofmt -l .            # no debe listar nada
go vet ./...
go test ./...                                   # unitarias
GM_INTEGRACION=1 go test -run Integracion ./... # integración contra MySQL

# Correr en desarrollo (detén antes el servicio, porque usa el mismo puerto)
sudo systemctl stop granmanzana-api
go run .

# Desplegar un cambio
go build -trimpath -o granmanzana-api . && sudo systemctl restart granmanzana-api
systemctl status granmanzana-api      # estado
journalctl -u granmanzana-api -f      # logs (una línea por petición)
```
- Servicio systemd: `/etc/systemd/system/granmanzana-api.service` (User=makina, Restart=on-failure, After=mysql.service, habilitado al arranque).
- Apache: `/etc/apache2/sites-available/000-default.conf` hace ProxyPass de `/api/` y `/granmanzana/` a `127.0.0.1:8080`. El respaldo del archivo original es `000-default.conf.bak-*`. Después de cualquier cambio, ejecutar `sudo apache2ctl configtest && sudo systemctl restart apache2`.
- MySQL: `mysql -e "..."` usa `~/.my.cnf`, configurado con el usuario de la app (`granmanzana_app`: SELECT/INSERT/UPDATE/DELETE solo sobre `Azure_makina`).
- Respaldo de la base antes de empezar: `~/respaldos/Azure_makina_20261023.sql`.

## Reglas
1. No modificar la estructura de la base ni borrar o cambiar datos existentes. No tocar /var/www/html, la configuración de PHP ni la clave de root de MySQL.
2. Antes de cada comando con sudo, decir para qué es. Si algo pide una clave que no se tiene, detenerse y pedirle al usuario que lo ejecute, dándole el comando exacto.
3. Nunca mostrar claves en la salida ni ponerlas en el código. Las credenciales van solo en `.env` (chmod 600), que está en `.gitignore`.
4. SQL siempre con placeholders `?`. Prohibido concatenar valores del usuario en el SQL (solo se concatenan fragmentos constantes).
5. Go solo con la librería estándar + `github.com/go-sql-driver/mysql` (`filippo.io/edwards25519` entra como dependencia indirecta del driver). Frontend en HTML, CSS y JS puros, sin CDN ni frameworks; el texto se inserta con `textContent`, nunca con `innerHTML`.
6. Si algo falla, leer el error, corregirlo y reintentar. Preguntar solo cuando haga falta una decisión del usuario.

Formato de respuestas: JSON UTF-8; errores `{"error":"..."}` con 400/404/405/409/500 (el detalle técnico va solo al log); listas vacías `[]`; NULL → `null` (escaneando en punteros); DATE → `"AAAA-MM-DD"`; TINYINT → bool.

## Base de datos (`Azure_makina`, MySQL 8.0)
10 tablas, sin triggers, vistas ni procedimientos:
- `productos` (categoria_id → categorias, proveedor_id → proveedores; `activo`, `stock_minimo`, `dias_alerta_temprana` = 120, `dias_alerta_liquidacion` = 60, textos de la ficha y `aviso_complemento`).
- `lotes` (producto_id, fecha_vencimiento, cantidad_inicial, cantidad_actual, costo_unitario). **El stock no está en productos**: es la suma de `lotes.cantidad_actual`.
- `categorias` (nombre UNIQUE, varchar 80), `proveedores` (acepta_devolucion), `etiquetas` (tipo ENUM necesidad/ingrediente/contraindicacion/otro, activa), `producto_etiqueta`.
- `ventas` (medio_pago ENUM efectivo/nequi/daviplata) y `detalle_venta` (`subtotal` es columna GENERADA).
- `bajas` (motivo y accion_previa son ENUM; `valor_perdido` es columna GENERADA). `usuarios` (rol propietario/empleado).

Para regenerar la colección de Postman, edita `docs_web/granmanzana.postman_collection.json`; cada petición lleva sus pruebas `pm.test`. Debe correr completa sin dejar datos en la base.

Endpoints: salud; categorías (GET lista, GET/{id}, POST, DELETE/{id} solo si no tiene productos); proveedores; productos (búsqueda, ficha, plan-fefo); etiquetas; recomendaciones; alertas (stock-bajo, vencimientos). El detalle está en `docs/API.md`.

Reglas de negocio implementadas:
- Stock vigente = `COALESCE(SUM(CASE WHEN l.fecha_vencimiento >= CURRENT_DATE THEN l.cantidad_actual ELSE 0 END), 0)` con LEFT JOIN lotes.
- FEFO: `cantidad_actual > 0 AND fecha_vencimiento >= CURRENT_DATE ORDER BY fecha_vencimiento, id`.
- Ventana de alerta: `cantidad_actual > 0 AND fecha_vencimiento <= CURRENT_DATE + INTERVAL p.dias_alerta_temprana DAY`.
- Nivel (en Go): `vencido` si días < 0, `liquidar` si días ≤ dias_alerta_liquidacion, si no `devolver`. `sugerirAccion` traduce el nivel en una acción según si el proveedor acepta devolución.
- `planificarFEFO` (función pura): reparte una cantidad entre lotes por fecha de vencimiento y luego por id, ignorando los vencidos o sin stock. La usa `plan-fefo` y debe reutilizarse en `POST /api/ventas`.
- Recomendaciones: necesidad (etiqueta de tipo necesidad) + `NOT EXISTS` de las etiquetas a evitar, con placeholders generados por `placeholders(n)`.

## Pendiente (en este orden)
1. `POST /api/ventas` con FEFO en una transacción: `SELECT ... FOR UPDATE` sobre los lotes, reutilizar `planificarFEFO`; una venta puede consumir varios lotes; 409 si no alcanza el stock.
2. Ingreso de lotes y registro de bajas.
3. Login con bcrypt (los usuarios de ejemplo tienen `password_hash = 'HASH_DE_EJEMPLO_CAMBIAR'`) y proteger los POST según el rol.
4. Reportes de ventas y pérdidas.
