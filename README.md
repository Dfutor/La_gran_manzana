# La Gran Manzana

Aplicación Go + MySQL 8/InnoDB para inventario por lotes FEFO, ventas y consulta orientativa de productos naturistas. HTML/CSS/JS básicos, sin frameworks ni CDN; todo se incluye en un binario. Usa la conexión y las diez tablas existentes: no ejecuta migraciones ni carga datos de ejemplo en producción.

## Servicios y descripción técnica

Todas las rutas usan JSON UTF-8 bajo `/api`. Salvo salud, login y logout, requieren sesión. **P** = propietario; **E/P** = empleado o propietario.

| Servicio | Rutas y permisos | Implementación |
|---|---|---|
| Salud | `GET /salud`, público | Verifica MySQL con timeout; informa estado, latencia y base. |
| Autenticación | `POST /login`, `POST /logout`; `GET /sesion` | Usuarios de MySQL, bcrypt, cookie HttpOnly/SameSite Strict, vencimiento de 8 h, límite de 10 intentos/IP/15 min. Sesiones en memoria: reiniciar exige ingresar nuevamente. Cada petición verifica estado, rol y hash actuales. |
| Usuarios | `GET/POST /usuarios`, `PUT /usuarios/{id}`; P | Alta, edición, roles, activación y cambio de contraseña; nunca devuelve hashes. Contraseña de 12–72 bytes; impide quitarse el propio acceso. |
| Categorías | `GET /categorias[/{id}]`, E/P; `POST/PUT/DELETE`, P | Nombre único; elimina solo categorías sin productos ni historial asociado. |
| Proveedores | `GET /proveedores`, E/P; `POST/PUT/DELETE /proveedores[/{id}]`, P | Contacto y aceptación de devoluciones; las referencias impiden eliminar proveedores en uso. |
| Productos | `GET /productos?q=...`, `GET /productos/{id}`, E/P; `POST/PUT/DELETE`, P | Búsqueda por nombre/marca/código; ficha, etiquetas y stock vigente/vencido. Producto y etiquetas se guardan en una transacción; DELETE desactiva y conserva historial. PUT reemplaza los datos comerciales; omitir `etiquetas` conserva sus asociaciones. |
| Etiquetas y consulta | `GET /etiquetas?tipo=...`, `/etiquetas/{id}/productos`, E/P; `POST/PUT/DELETE`, P | Necesidad, ingrediente, contraindicación u otro; asociación muchos a muchos; desactivación lógica. |
| Recomendaciones | `GET /recomendaciones?necesidad=ID&evitar=ID,ID&disponibles=1`, E/P | Filtra etiquetas de necesidad y excluye contraindicaciones seleccionadas; muestra ficha/aviso educativo. No incorpora el chatbot previsto para una fase futura. |
| Lotes | `GET /lotes?producto_id=ID`, `POST /lotes`, E/P; `PUT /lotes/{id}`, P | Ingreso con factura, vencimiento, unidades y costo; rechaza ingresos vencidos. Edición de código/factura/fecha; cantidades cambian mediante ingresos, ventas y bajas. |
| FEFO | `GET /productos/{id}/plan-fefo?cantidad=N`, E/P | Simula asignación por vencimiento e ID; excluye agotados/vencidos. Stock insuficiente: 409. |
| Ventas | `GET/POST /ventas`, `GET /ventas/{id}`, E/P | Efectivo/Nequi/Daviplata; transacción con `SELECT FOR UPDATE`, consume uno o varios lotes FEFO y guarda encabezado/detalles. Calcula total en centavos exactos; permite precio de oferta; conserva precio histórico. Error: rollback completo. Usuario tomado de la sesión. |
| Bajas | `GET/POST /bajas`, E/P | Vencimiento, deterioro, obsequio, devolución u otro; guarda acción previa/costo/usuario y descuenta el lote bajo bloqueo transaccional. Valor perdido generado por MySQL. |
| Alertas | `GET /alertas/stock-bajo`, `/alertas/vencimientos`, E/P | Stock vigente ≤ mínimo; vencido/liquidar/devolver según umbrales del producto y proveedor. Proceso automático cada minuto registra cambios en journald; aviso global del navegador cada 30 s. No envía correo/SMS. |
| Reportes | `GET /reportes/ventas[?agrupar=producto]`, `/reportes/perdidas`, `/reportes/rotacion`; P | Ventas por día/pago o producto; pérdidas por motivo/acción; unidades, promedio diario, cobertura y reposición estimada a 30 días. Incluye productos sin ventas; no presupone stock histórico. |
| Interfaz y documentación | `/granmanzana/`; `/granmanzana/docs/` | Formularios y tablas básicos con login; administración/reportes para propietario. Docs y Postman conservan HTTP Basic independiente (`DOCS_USER/PASS`). |

Listas de lotes/ventas/bajas: `limite=1..500` (100 por defecto), `offset>=0`. Ventas, bajas y reportes: `desde=AAAA-MM-DD&hasta=AAAA-MM-DD` inclusivos; por defecto últimos 30 días, máximo 367 días. Granel: una unidad equivale a **100 g**, conforme al esquema INT actual. Reposición estimada = `max(mínimo, ceil(promedio_diario × 30)) − stock_vigente`, mínimo cero. Pérdidas suma todas las bajas por separado, incluidas devoluciones y obsequios.

## Configuración y ejecución

Requiere Go **1.27.3**, MySQL **8.0+** y las tablas existentes. Dependencias: driver MySQL y `golang.org/x/crypto/bcrypt`. SQL parametrizado, cuerpos JSON limitados, errores técnicos solo en logs, timeouts y cierre ordenado.

```bash
cp .env.example .env
chmod 600 .env
# Editar .env con la conexión existente. No sobrescribir el .env del servidor.
go mod download
go test ./...
go vet ./...
go build -trimpath -o granmanzana-api .
./granmanzana-api
```

`APP_ADDR=127.0.0.1:8080` mantiene el proxy Apache actual. `COOKIE_SECURE=false` permite el HTTP actual; con HTTPS usar `COOKIE_SECURE=true`. Las cookies se usan desde el mismo origen; el API rechaza escrituras desde un Origin diferente. `.env` nunca se versiona.

### Primer acceso

Las contraseñas `HASH_DE_EJEMPLO_CAMBIAR` no permiten ingresar. Desde la carpeta del proyecto, crea el primer propietario **con un correo nuevo** o configura la clave de un usuario ya existente. La contraseña se lee por stdin y se guarda con bcrypt; no se imprime ni se pasa como argumento.

```bash
read -r -s -p 'Nueva contraseña (12–72 bytes): ' GM_CLAVE
printf '\n'
printf '%s\n' "$GM_CLAVE" | ./granmanzana-api -crear-propietario propietario@mitienda.com
unset GM_CLAVE
```

Si el propietario ya existe, sustituye la última opción por `-restablecer-clave correo-existente@mitienda.com`. Esta opción cambia solo su hash: conserva rol/estado; úsala para las cuentas de ejemplo existentes. Crear el primer propietario se bloquea si ya existe uno activo con bcrypt. Después, administra empleados y claves desde **Administración → usuarios**. Sin registro público ni credenciales predeterminadas.

## Actualizar producción: pull y reinicio

Según la documentación del repositorio: Ubuntu/Azure, usuario `makina`, servicio `granmanzana-api`, Apache como proxy y MySQL `Azure_makina`. Ejecuta en el servidor; no requiere recrear tablas, cambiar el proxy ni tocar PHP. No ejecutes `docs/esquema.sql` en producción: contiene `DROP TABLE`.

```bash
ssh makina@158.158.17.188
# Consultar la carpeta real del servicio y entrar en ella:
systemctl show granmanzana-api -p WorkingDirectory -p ExecStart
cd /ruta/real/del/proyecto
# Verificar que no haya cambios locales pendientes; conservar .env.
git status --short
# Respaldo SQL (usa las credenciales de ~/.my.cnf ya configuradas):
mkdir -p ~/respaldos
mysqldump --no-tablespaces --single-transaction Azure_makina > ~/respaldos/Azure_makina_$(date +%Y%m%d_%H%M%S).sql
git pull --ff-only origin main
/usr/local/go/bin/go mod download
/usr/local/go/bin/go test ./...
/usr/local/go/bin/go vet ./...
/usr/local/go/bin/go build -trimpath -o granmanzana-api.new .
# Conservar el binario anterior y reemplazarlo solo cuando la compilación pase:
cp -p granmanzana-api granmanzana-api.previous
mv granmanzana-api.new granmanzana-api
```

Configura el primer acceso con el comando anterior antes de reiniciar, si aún no hay una cuenta con bcrypt. El siguiente `sudo` reinicia **solo la aplicación**, manteniendo MySQL/Apache:

```bash
sudo systemctl restart granmanzana-api
systemctl status granmanzana-api --no-pager
journalctl -u granmanzana-api -n 50 --no-pager
curl --fail http://127.0.0.1:8080/api/salud
```

Abre `http://158.158.17.188/granmanzana/`, ingresa y comprueba catálogo, alertas y reportes; registra movimientos reales solo cuando correspondan. El restart cierra las sesiones existentes. Si falla el arranque, consulta el log; para volver al binario anterior, `cp -p granmanzana-api.previous granmanzana-api.rollback`, `mv granmanzana-api.rollback granmanzana-api` y `sudo systemctl restart granmanzana-api` (restaura el servicio anterior; no cambia datos).

### Montar el servicio por primera vez

Si **ya existe**, conserva su unidad. Si el servidor no lo tiene, clona en `/home/makina/La_gran_manzana`, configura `.env`, compila el binario y crea el propietario. La plantilla [deploy/granmanzana-api.service](deploy/granmanzana-api.service) usa esa carpeta; ajusta usuario/rutas si difieren. Estos comandos con `sudo` instalan la unidad y habilitan su inicio automático:

```bash
sudo install -m 644 deploy/granmanzana-api.service /etc/systemd/system/granmanzana-api.service
sudo systemctl daemon-reload
sudo systemctl enable --now granmanzana-api
```

El proxy existente debe conservar `/api/` y `/granmanzana/` hacia `http://127.0.0.1:8080` y el host público (`ProxyPreserveHost On` o el `X-Forwarded-Host` que Apache agrega por defecto). El API solo confía en ese encabezado desde un proxy conectado por loopback. Para un servidor nuevo consulta [deploy/apache.conf.example](deploy/apache.conf.example); integra las directivas en su VirtualHost sin reemplazar otros sitios. Después de editar Apache, `sudo apache2ctl configtest` valida la configuración y `sudo systemctl reload apache2` aplica el cambio.

## Verificación y alcance

```bash
gofmt -l .
go test -race ./...
go vet ./...
# Integración SOLO en una base desechable cuyo nombre empiece por granmanzana_test:
GM_INTEGRACION=1 DB_NAME=granmanzana_test go test -race -count=1 ./...
```

La integración requiere el esquema y un catálogo mínimo: [docs/PRUEBAS.md](docs/PRUEBAS.md). Verifica bcrypt/permisos, FEFO entre lotes, rollback, venta concurrente, bajas, precio histórico y reportes. La validación con personal de la tienda y el despliegue real se realizan en el servidor; no son pruebas simuladas. Matriz del documento entregado: [docs/REQUISITOS.md](docs/REQUISITOS.md). Contratos y ejemplos: [docs/API.md](docs/API.md).
