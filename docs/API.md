# API «La Gran Manzana»

Servicio REST del inventario por lotes (FEFO) y de la consulta/recomendación de productos
de la tienda naturista «La Gran Manzana» (Melgar, Tolima). Proyecto de Calidad de Software.

| | |
|---|---|
| **URL base del API** | `http://158.158.17.188/api` |
| **Frontend de prueba** | `http://158.158.17.188/granmanzana/` (básico, con *Probador de API*) |
| **Documentación privada + colección Postman** | `http://158.158.17.188/granmanzana/docs/` (usuario y contraseña) |
| **Formato** | JSON en UTF-8 |
| **Autenticación** | Sesión con cookie HttpOnly después de `POST /api/login`; escrituras de catálogo, usuarios y reportes reservados al propietario. |
| **CORS** | `Access-Control-Allow-Origin: *` |

## Login y permisos

`POST /api/login` recibe `{"email":"correo@tienda.com","password":"contraseña"}` y devuelve los datos públicos del usuario; guarda una cookie de sesión de ocho horas. `GET /api/sesion` consulta el acceso actual; `POST /api/logout` lo revoca. Reiniciar cierra todas las sesiones. Contraseñas almacenadas con bcrypt; los hashes de marcador no permiten ingresar. El primer acceso se configura por consola según README.

Ejemplo para curl, con un archivo temporal que contenga el JSON del login (permisos 600, eliminar después):

```bash
curl -c cookies.txt -H 'Content-Type: application/json' --data-binary @login.json http://127.0.0.1:8080/api/login
curl -b cookies.txt http://127.0.0.1:8080/api/productos
curl -b cookies.txt -X POST http://127.0.0.1:8080/api/logout
```

Añadir `-b cookies.txt` a los ejemplos siguientes. `401`: falta sesión/está revocada; `403`: falta rol o Origin no permitido; `429`: límite de login. Empleado y propietario consultan catálogo/alertas/historial y registran ventas, lotes y bajas. Solo propietario modifica catálogo, corrige lotes, administra usuarios y consulta reportes. Las cookies se usan en el mismo origen; CORS no habilita sesiones entre sitios.

## Movimientos, usuarios y reportes

| Método/ruta | Entrada y resultado |
|---|---|
| `GET/POST /api/usuarios`, `PUT /api/usuarios/{id}` | `{nombre,email,password,rol,estado}`; rol propietario/empleado, estado activo/inactivo. Password obligatorio al crear, opcional al editar, 12–72 bytes. GET omite hashes. |
| `GET /api/lotes?producto_id=ID` | Todos los lotes, incluidos vencidos/agotados, en orden de vencimiento/ID. |
| `POST /api/lotes` | `{producto_id,codigo_lote?,numero_factura?,fecha_vencimiento,cantidad,costo_unitario}`; cantidad positiva, fecha vigente y costo con hasta dos decimales. 201 con ID/cantidad actual. |
| `PUT /api/lotes/{id}` | `{codigo_lote?,numero_factura?,fecha_vencimiento}`; no modifica cantidades ni costos históricos. |
| `POST /api/ventas` | `{medio_pago,detalles:[{producto_id,cantidad,precio_unitario?}]}`; hasta 100 productos únicos; cantidades 1..1000000. Precio opcional de oferta; si se omite usa catálogo. Total calculado por servidor en centavos exactos. |
| `GET /api/ventas`, `GET /api/ventas/{id}` | Encabezados paginados y detalle con precio histórico. La distribución FEFO se devuelve al crear la venta; no se persiste porque el esquema actual no contiene venta-lote. |
| `POST /api/bajas` | `{lote_id,cantidad,motivo,accion_previa?}`; producto/costo/usuario derivados del lote y sesión. Acción por defecto ninguna. 201 con ID/valor perdido/stock restante. |
| `GET /api/bajas` | Historial de salidas con motivo/acción/costo/usuario. |
| `GET /api/reportes/ventas?agrupar=dia` | Ventas/total por día y medio de pago. `agrupar=producto`: ranking de unidades y total vendido. |
| `GET /api/reportes/perdidas` | Unidades y valor de bajas por motivo y acción previa. |
| `GET /api/reportes/rotacion` | Stock actual, unidades/total del periodo, promedio diario, días de cobertura, sugerencia de compra a 30 días y estado sin_movimiento/reponer/con_movimiento. |

Historial y reportes admiten `desde`/`hasta` (inclusivos, AAAA-MM-DD, últimos 30 días por defecto; máximo 367 días). Lotes/ventas/bajas admiten `limite` (1..500, predeterminado 100) y `offset` (>=0). Las estimaciones de rotación combinan ventas del periodo con stock vigente actual; no calculan rotación contable sobre inventario promedio histórico. Importes nuevos aceptan JSON numérico o texto decimal, nunca negativos ni más de dos decimales. Granel: unidades enteras de 100 g.

Venta de ejemplo:

```json
{"medio_pago":"nequi","detalles":[{"producto_id":1,"cantidad":3}]}
```

Baja de ejemplo:

```json
{"lote_id":1,"cantidad":1,"motivo":"deterioro","accion_previa":"rebaja"}
```

Motivos: vencimiento/deterioro/obsequio/devolucion/otro. Acciones: ninguna/venta_destacada/rebaja/combo/devolucion_distribuidor/obsequio. Una baja por vencimiento exige lote ya vencido. Ventas y bajas descuentan lotes en transacciones con bloqueo: stock insuficiente devuelve 409 y revierte todos los cambios.

## Convenciones

- Las listas vacías se devuelven como `[]`, nunca `null`.
- Los campos sin valor en la base se devuelven como `null`.
- Las fechas tipo DATE van como texto `"AAAA-MM-DD"`.
- Los indicadores (`es_granel`, `acepta_devolucion`, …) van como `true`/`false`.
- **Stock vigente** = suma de `cantidad_actual` de los lotes **no vencidos** del producto.
- **FEFO** (*First Expired, First Out*): los lotes con existencias y sin vencer se ordenan por fecha de vencimiento; se vende primero el que vence primero.
- El catálogo y las recomendaciones ocultan productos inactivos; el historial conserva sus movimientos.
- La ficha del producto es orientativa (Decreto 3249 de 2006) y siempre incluye `aviso_complemento`.

## Endpoints

| Método | Ruta | Descripción |
|---|---|---|
| GET | `/api/salud` | Estado del API y de MySQL |
| GET | `/api/categorias` | Lista de categorías con su número de productos |
| GET | `/api/categorias/{id}` | Una categoría |
| POST | `/api/categorias` | Crea una categoría |
| PUT | `/api/categorias/{id}` | Reemplaza el nombre de una categoría |
| DELETE | `/api/categorias/{id}` | Elimina una categoría **sin productos** |
| GET | `/api/proveedores` | Proveedores y si aceptan devolución |
| POST/PUT/DELETE | `/api/proveedores[/{id}]` | Administra proveedores (DELETE exige que no tengan productos) |
| POST | `/api/productos` | Crea un producto activo |
| PUT | `/api/productos/{id}` | Actualiza un producto activo y, opcionalmente, sus etiquetas |
| DELETE | `/api/productos/{id}` | Desactiva un producto (baja lógica) |
| GET | `/api/productos?q=texto` | Busca productos activos por nombre, marca o código de barras |
| GET | `/api/productos/{id}` | Ficha, etiquetas y lotes FEFO de un producto |
| GET | `/api/productos/{id}/plan-fefo?cantidad=N` | Simula de qué lotes saldría una venta (no modifica nada) |
| GET | `/api/etiquetas?tipo=…` | Etiquetas activas (filtro opcional por tipo) |
| POST | `/api/etiquetas` | Crea una etiqueta |
| PUT | `/api/etiquetas/{id}` | Actualiza una etiqueta (incluida `activa`) |
| DELETE | `/api/etiquetas/{id}` | Desactiva una etiqueta (baja lógica) |
| GET | `/api/etiquetas/{id}/productos` | Productos de una etiqueta |
| GET | `/api/recomendaciones?necesidad=ID&evitar=ID,ID&disponibles=1` | Recomendación por necesidad excluyendo contraindicaciones |
| GET | `/api/alertas/stock-bajo` | Productos con stock vigente ≤ stock mínimo |
| GET | `/api/alertas/vencimientos` | Lotes dentro de la ventana de alerta, con su nivel |

---

### Operaciones de escritura

Todas las escrituras reciben `Content-Type: application/json`, rechazan campos
desconocidos y devuelven `400` para cuerpos inválidos, `404` si el recurso no
existe y `409` para duplicados o referencias que impedirían borrar. Las
respuestas de creación son `201` con `Location`; los borrados exitosos son
`204` sin cuerpo.

```json
{"nombre":"Proveedor Andino","telefono":"3005551234","acepta_devolucion":true}
```
Es el cuerpo de `POST /api/proveedores` y `PUT /api/proveedores/{id}`.
Categorías usan únicamente `{"nombre":"..."}`. Para etiquetas:
`{"nombre":"Digestión","tipo":"necesidad","activa":true}`; los tipos válidos
son `necesidad`, `ingrediente`, `contraindicacion` y `otro`. Las etiquetas y
productos se desactivan, no se eliminan físicamente, para conservar el
historial y las claves foráneas.

El cuerpo de producto requiere `nombre` y `categoria_id` y admite:

```json
{
  "codigo_barras":"770000000001", "nombre":"Producto", "marca":"Marca",
  "presentacion":"Caja x 30", "categoria_id":1, "proveedor_id":2,
  "precio_venta":25000, "margen_porcentaje":30, "stock_minimo":2,
  "dias_alerta_temprana":120, "dias_alerta_liquidacion":60, "es_granel":false,
  "para_que_sirve":"...", "modo_toma":"...", "contraindicaciones":"...",
  "ingredientes":"...", "aviso_complemento":"...", "etiquetas":[3,7]
}
```
`etiquetas` solo puede contener IDs de etiquetas activas; `categoria_id` y
`proveedor_id` deben referir registros existentes. En `PUT`, enviar
`etiquetas` reemplaza las asociaciones actuales.

---

### GET /api/salud

```bash
curl -i http://158.158.17.188/api/salud
```
```json
{
  "api": "ok",
  "base": "Azure_makina",
  "hora_servidor": "2026-10-23T01:29:19-05:00",
  "mysql": "8.0.46-0ubuntu0.24.04.4"
}
```
Si MySQL no responde: **500** con `{"api":"ok","error":"sin conexión con MySQL"}`.

### GET /api/categorias

```bash
curl -i http://158.158.17.188/api/categorias
```
```json
[
  { "id": 5, "nombre": "Colágeno" },
  { "id": 8, "nombre": "Gomitas" }
]
```

### POST /api/categorias

Cuerpo: `{"nombre": "..."}`. El nombre se recorta y debe tener entre 1 y 80 caracteres.

```bash
curl -i -X POST http://158.158.17.188/api/categorias \
  -H 'Content-Type: application/json' \
  -d '{"nombre":"Superalimentos"}'
```
**201 Created** (con cabecera `Location`):
```json
{ "id": 15, "nombre": "Superalimentos" }
```

| Caso | Código | Respuesta |
|---|---|---|
| Cuerpo vacío | 400 | `{"error":"el cuerpo está vacío; se espera {\"nombre\":\"...\"}"}` |
| JSON mal formado o con campos extra | 400 | `{"error":"JSON inválido; se espera {\"nombre\":\"...\"}"}` |
| Nombre vacío o solo espacios | 400 | `{"error":"el nombre es obligatorio"}` |
| Más de 80 caracteres | 400 | `{"error":"el nombre no puede superar 80 caracteres"}` |
| Ya existe (sin distinguir mayúsculas ni tildes) | 409 | `{"error":"ya existe una categoría con ese nombre"}` |

### GET /api/productos?q=texto

`q` es opcional; sin `q` devuelve todos los productos activos. Busca coincidencias parciales en nombre, marca o código de barras.

```bash
curl -i 'http://158.158.17.188/api/productos?q=gel'
```
```json
[
  {
    "id": 14,
    "codigo_barras": "7703924196898",
    "nombre": "Gel energético Limón",
    "marca": "IronPeak",
    "presentacion": "Caja x 12",
    "categoria_id": 7,
    "categoria": "Suplementación deportiva",
    "precio_venta": 84000,
    "es_granel": false,
    "stock": 1,
    "stock_minimo": 5,
    "stock_bajo": true
  }
]
```

### GET /api/productos/{id}/plan-fefo?cantidad=N

Simula una venta de N unidades: reparte la cantidad entre los lotes vendibles empezando por el que vence primero.
**No modifica el inventario**; es la base del futuro `POST /api/ventas`.

```bash
curl -i "http://158.158.17.188/api/productos/1/plan-fefo?cantidad=40"
```
```json
{
  "producto_id": 1,
  "producto": "Llantén",
  "cantidad_solicitada": 40,
  "stock_vigente": 52,
  "lotes_usados": 2,
  "asignacion": [
    { "lote_id": 1, "codigo_lote": "L25E877", "fecha_vencimiento": "2027-04-20", "dias_restantes": 179, "disponible": 32, "tomar": 32, "quedaria": 0 },
    { "lote_id": 2, "codigo_lote": "L24E695", "fecha_vencimiento": "2028-08-29", "dias_restantes": 676, "disponible": 20, "tomar": 8, "quedaria": 12 }
  ],
  "nota": "Simulación: no modifica el inventario."
}
```
- **400** si falta `cantidad` o no es un entero entre 1 y 1 000 000.
- **404** si el producto no existe.
- **409** `{"error":"stock insuficiente: se pidieron 60 y hay 52 disponibles en lotes vigentes"}`.

### GET /api/productos/{id}

```bash
curl -i http://158.158.17.188/api/productos/1
```
```json
{
  "ficha": {
    "id": 1,
    "codigo_barras": "7706582802582",
    "nombre": "Llantén",
    "marca": "Vida Verde",
    "presentacion": "Cápsulas x 60",
    "categoria_id": 1,
    "categoria": "Hierbas",
    "precio_venta": 33100,
    "es_granel": false,
    "stock": 52,
    "stock_minimo": 3,
    "stock_bajo": false,
    "dias_alerta_temprana": 120,
    "dias_alerta_liquidacion": 60,
    "para_que_sirve": "Complemento dietario que apoya tradicionalmente la digestión.",
    "modo_toma": "1 cápsula 2 veces al día con las comidas, o según indique el rótulo.",
    "contraindicaciones": "Consultar al médico si toma medicamentos.",
    "ingredientes": "Extracto seco de llantén",
    "aviso_complemento": "Es un complemento, no un medicamento. Ante algo serio, consulte a su médico.",
    "proveedor": {
      "id": 2,
      "nombre": "Herbolario Tolima Mayorista",
      "telefono": "38594770284",
      "acepta_devolucion": true
    }
  },
  "etiquetas": [
    { "id": 5, "nombre": "Digestión", "tipo": "necesidad" },
    { "id": 12, "nombre": "Consultar si toma medicamentos", "tipo": "contraindicacion" }
  ],
  "lotes": [
    { "id": 1, "codigo_lote": "L25E877", "fecha_vencimiento": "2027-04-20", "cantidad_actual": 32, "dias_restantes": 179 },
    { "id": 2, "codigo_lote": "L24E695", "fecha_vencimiento": "2028-08-29", "cantidad_actual": 20, "dias_restantes": 676 }
  ]
}
```
- `lotes` viene en orden FEFO y solo incluye lotes con existencias y sin vencer.
- `proveedor` es `null` si el producto no tiene proveedor.
- **404** `{"error":"producto no encontrado"}` si no existe o está inactivo.
- **400** si el id no es un entero positivo.

### GET /api/etiquetas?tipo=necesidad

`tipo` es opcional: `necesidad`, `contraindicacion`, `ingrediente` u `otro`. Un tipo distinto devuelve **400**.

```bash
curl -i 'http://158.158.17.188/api/etiquetas?tipo=necesidad'
```
```json
[
  { "id": 7, "nombre": "Articulaciones", "tipo": "necesidad", "cantidad_productos": 1 },
  { "id": 10, "nombre": "Control de peso", "tipo": "necesidad", "cantidad_productos": 1 }
]
```

### GET /api/etiquetas/{id}/productos

Productos activos con esa etiqueta. Primero aparecen los que tienen existencias.

```bash
curl -i http://158.158.17.188/api/etiquetas/3/productos
```
```json
{
  "etiqueta": { "id": 3, "nombre": "Para el gimnasio", "tipo": "necesidad" },
  "productos": [
    {
      "id": 13, "codigo_barras": "7709518380125", "nombre": "Citrulina malato",
      "marca": "IronPeak", "presentacion": "300 g", "categoria_id": 7,
      "categoria": "Suplementación deportiva", "precio_venta": 82000,
      "es_granel": false, "stock": 4, "stock_minimo": 5, "stock_bajo": true
    }
  ]
}
```
**404** `{"error":"etiqueta no encontrada"}` si no existe o está inactiva.

### GET /api/recomendaciones?necesidad=ID&evitar=ID,ID&disponibles=1

Recomienda productos para una necesidad del cliente y **excluye** los que tengan alguna de las etiquetas en `evitar`
(por ejemplo, 11 = embarazo, 13 = hipertensión). Con `disponibles=1` solo devuelve productos con existencias.

```bash
curl -i "http://158.158.17.188/api/recomendaciones?necesidad=3&evitar=13&disponibles=1"
```
```json
{
  "necesidad": { "id": 3, "nombre": "Para el gimnasio", "tipo": "necesidad" },
  "evitando": [ { "id": 13, "nombre": "Precaución en hipertensos", "tipo": "contraindicacion" } ],
  "productos": [ { "id": 13, "nombre": "Citrulina malato", "stock": 4, "...": "..." } ],
  "aviso": "Información orientativa (Decreto 3249 de 2006). ..."
}
```
- **400** si falta `necesidad`, si no es numérica, si la etiqueta no es de tipo necesidad o si `evitar` está mal formado (máximo 20 ids).
- **404** si la necesidad o alguna etiqueta en `evitar` no existe.

### DELETE /api/categorias/{id}

```bash
curl -i -X DELETE http://158.158.17.188/api/categorias/15
```
**204** sin cuerpo si se eliminó. **409** si la categoría tiene productos (las categorías con historial están protegidas). **404** si no existe.

### GET /api/alertas/stock-bajo

Productos activos con stock vigente menor o igual a su `stock_minimo`, ordenados de menor a mayor stock. Cada elemento tiene la misma forma que en `/api/productos`, más `faltante_para_minimo`.

```bash
curl -i http://158.158.17.188/api/alertas/stock-bajo
```

### GET /api/alertas/vencimientos

Lotes con existencias que vencen dentro de la ventana de alerta del producto
(`fecha_vencimiento <= hoy + dias_alerta_temprana`), ordenados por fecha de vencimiento.

| `nivel` | Regla | Acción sugerida |
|---|---|---|
| `vencido` | días restantes < 0 | Retirar y dar de baja |
| `liquidar` | días restantes ≤ `dias_alerta_liquidacion` | Rebaja, combo o venta destacada |
| `devolver` | resto de la ventana | Gestionar devolución si el proveedor la acepta |

```bash
curl -i http://158.158.17.188/api/alertas/vencimientos
```
```json
[
  {
    "lote_id": 26,
    "codigo_lote": "L26E027",
    "producto_id": 18,
    "producto": "Arándanos deshidratados",
    "marca": "Granel Natural",
    "presentacion": "Por 100 g",
    "fecha_vencimiento": "2026-10-29",
    "cantidad_actual": 16,
    "costo_unitario": 4500,
    "valor_en_riesgo": 72000,
    "dias_restantes": 6,
    "dias_alerta_liquidacion": 60,
    "nivel": "liquidar",
    "accion_sugerida": "Liquidar: rebaja, combo o venta destacada",
    "proveedor": "Granos y Semillas del Centro",
    "proveedor_telefono": "3999923568",
    "acepta_devolucion": false
  }
]
```

## Códigos de respuesta

| Código | Significado |
|---|---|
| 200 | Consulta correcta |
| 201 | Recurso creado |
| 204 | Respuesta a `OPTIONS` (CORS) |
| 400 | Datos inválidos (JSON mal formado, campo vacío, id o tipo inválido) |
| 204 | Eliminado (sin cuerpo) |
| 404 | Producto, categoría, etiqueta o ruta inexistente |
| 405 | Método HTTP no soportado |
| 409 | Conflicto: duplicado, categoría en uso o stock insuficiente |
| 500 | Error interno (el detalle técnico queda solo en el log; el mensaje trae una referencia) |

Todos los errores tienen la forma `{"error": "mensaje"}`. Cada respuesta trae la cabecera `X-Request-Id`, que también aparece en el log.

## Pruebas con Postman

La colección `granmanzana.postman_collection.json` (incluye login con variables email/password) se descarga desde la
documentación privada. Se importa en Postman y se ejecuta con **Run collection**, en orden. Crea sus propias categorías de prueba
y las elimina al final.

## Seguridad

- Todas las consultas usan parámetros (`?`); los valores del usuario nunca se concatenan en el SQL.
  Un intento como `{"nombre":"x'); DROP TABLE categorias; --"}` se guarda como texto literal.
- El API se conecta con un usuario de MySQL que solo tiene SELECT, INSERT, UPDATE y DELETE sobre la base del proyecto.
- El API solo escucha en `127.0.0.1:8080`; al público llega a través de Apache (proxy inverso).
