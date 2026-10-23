# API «La Gran Manzana»

Servicio REST del inventario por lotes (FEFO) y de la consulta/recomendación de productos
de la tienda naturista «La Gran Manzana» (Melgar, Tolima). Proyecto de Calidad de Software.

| | |
|---|---|
| **URL base del API** | `http://158.158.17.188/api` |
| **Frontend de prueba** | `http://158.158.17.188/granmanzana/` (incluye un *Probador de API*) |
| **Formato** | JSON en UTF-8 |
| **Autenticación** | Ninguna por ahora (versión de prueba) |
| **CORS** | `Access-Control-Allow-Origin: *` |

## Convenciones

- Las listas vacías se devuelven como `[]`, nunca `null`.
- Los campos sin valor en la base se devuelven como `null`.
- Las fechas tipo DATE van como texto `"AAAA-MM-DD"`.
- Los indicadores (`es_granel`, `acepta_devolucion`, …) van como `true`/`false`.
- **Stock vigente** = suma de `cantidad_actual` de los lotes **no vencidos** del producto.
- **FEFO** (*First Expired, First Out*): los lotes con existencias y sin vencer se ordenan por fecha de vencimiento; se vende primero el que vence primero.
- Los productos inactivos (`activo = 0`) no aparecen en ningún endpoint.
- La ficha del producto es orientativa (Decreto 3249 de 2006) y siempre incluye `aviso_complemento`.

## Endpoints

| Método | Ruta | Descripción |
|---|---|---|
| GET | `/api/salud` | Estado del API y de MySQL |
| GET | `/api/categorias` | Lista de categorías |
| POST | `/api/categorias` | Crea una categoría |
| GET | `/api/productos?q=texto` | Busca productos activos por nombre, marca o código de barras |
| GET | `/api/productos/{id}` | Ficha, etiquetas y lotes FEFO de un producto |
| GET | `/api/etiquetas?tipo=…` | Etiquetas activas (filtro opcional por tipo) |
| GET | `/api/etiquetas/{id}/productos` | Productos recomendados para una etiqueta |
| GET | `/api/alertas/stock-bajo` | Productos con stock vigente ≤ stock mínimo |
| GET | `/api/alertas/vencimientos` | Lotes dentro de la ventana de alerta, con su nivel |

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

### GET /api/alertas/stock-bajo

Productos activos con stock vigente menor o igual a su `stock_minimo`, ordenados de menor a mayor stock. Cada elemento tiene la misma forma que en `/api/productos`.

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
    "dias_restantes": 6,
    "dias_alerta_liquidacion": 60,
    "nivel": "liquidar",
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
| 404 | Producto, etiqueta o ruta inexistente |
| 405 | Método HTTP no soportado |
| 409 | Conflicto: el registro ya existe |
| 500 | Error interno (el detalle técnico queda solo en el log del servidor) |

Todos los errores tienen la forma `{"error": "mensaje"}`.

## Seguridad

- Todas las consultas usan parámetros (`?`); los valores del usuario nunca se concatenan en el SQL.
  Un intento como `{"nombre":"x'); DROP TABLE categorias; --"}` se guarda como texto literal.
- El API se conecta con un usuario de MySQL que solo tiene SELECT, INSERT, UPDATE y DELETE sobre la base del proyecto.
- El API solo escucha en `127.0.0.1:8080`; al público llega a través de Apache (proxy inverso).
