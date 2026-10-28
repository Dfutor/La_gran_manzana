-- SOLO en una base desechable recién creada cuyo nombre empiece por granmanzana_test.
-- Cargar después de esquema.sql. Sin contraseñas ni datos de producción.
INSERT INTO categorias(nombre) VALUES('Categoría prueba');
INSERT INTO proveedores(nombre,acepta_devolucion) VALUES('Proveedor prueba',1);
INSERT INTO etiquetas(nombre,tipo) VALUES('Necesidad prueba','necesidad'),('Contraindicación prueba','contraindicacion');
INSERT INTO productos(nombre,categoria_id,proveedor_id,precio_venta,stock_minimo) VALUES('Producto fixture',1,1,1000,3),('Producto sin stock',1,1,2000,2);
INSERT INTO producto_etiqueta(producto_id,etiqueta_id) VALUES(1,1),(2,1),(2,2);
INSERT INTO lotes(producto_id,fecha_vencimiento,cantidad_inicial,cantidad_actual,costo_unitario) VALUES(1,CURRENT_DATE+INTERVAL 10 DAY,10,10,500),(1,CURRENT_DATE-INTERVAL 1 DAY,5,5,500);
