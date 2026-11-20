-- Control de acceso basado en roles (RBAC) para La Gran Manzana.
--
-- Agrega siete tablas (control de acceso y recomendaciones del panel); no modifica ni borra tablas o datos existentes y se puede
-- ejecutar varias veces (solo crea lo que falta). Requiere un usuario MySQL con
-- CREATE y REFERENCES; el usuario de la aplicación sigue necesitando solo
-- SELECT/INSERT/UPDATE/DELETE:
--
--   mysql -u <administrador> -p Azure_makina < docs/migracion_rbac.sql
--
-- Los datos los carga la aplicación al iniciar:
--   * sincroniza la tabla permisos con el catálogo definido en rbac.go;
--   * crea el rol del sistema «Propietario» con todos los permisos;
--   * la primera vez crea el rol «Empleado» (mismo acceso que el rol fijo
--     anterior) y asigna cada usuario según su columna usuarios.rol.
-- La columna usuarios.rol se conserva y se mantiene sincronizada para poder
-- volver al binario anterior sin perder accesos.

CREATE TABLE IF NOT EXISTS `permisos` (
  `nombre` varchar(60) NOT NULL,
  `modulo` varchar(40) NOT NULL,
  `descripcion` varchar(200) NOT NULL,
  PRIMARY KEY (`nombre`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `roles` (
  `id` int NOT NULL AUTO_INCREMENT,
  `nombre` varchar(60) NOT NULL,
  `descripcion` varchar(255) NOT NULL DEFAULT '',
  `es_sistema` tinyint(1) NOT NULL DEFAULT '0',
  `activo` tinyint(1) NOT NULL DEFAULT '1',
  `fecha_creacion` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `fecha_actualizacion` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_roles_nombre` (`nombre`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `rol_permisos` (
  `rol_id` int NOT NULL,
  `permiso` varchar(60) NOT NULL,
  PRIMARY KEY (`rol_id`,`permiso`),
  KEY `idx_rol_permisos_permiso` (`permiso`),
  CONSTRAINT `fk_rol_permisos_rol` FOREIGN KEY (`rol_id`) REFERENCES `roles` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_rol_permisos_permiso` FOREIGN KEY (`permiso`) REFERENCES `permisos` (`nombre`) ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Un rol asignado no se puede borrar (RESTRICT): primero se quita o se desactiva.
CREATE TABLE IF NOT EXISTS `usuario_roles` (
  `usuario_id` int NOT NULL,
  `rol_id` int NOT NULL,
  `asignado_por` int DEFAULT NULL,
  `fecha_asignacion` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`usuario_id`,`rol_id`),
  KEY `idx_usuario_roles_rol` (`rol_id`),
  KEY `idx_usuario_roles_asignado_por` (`asignado_por`),
  CONSTRAINT `fk_usuario_roles_usuario` FOREIGN KEY (`usuario_id`) REFERENCES `usuarios` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_usuario_roles_rol` FOREIGN KEY (`rol_id`) REFERENCES `roles` (`id`),
  CONSTRAINT `fk_usuario_roles_asignado_por` FOREIGN KEY (`asignado_por`) REFERENCES `usuarios` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Registro de cambios de acceso. La aplicación solo inserta; nunca edita ni borra.
-- actor_email conserva quién hizo el cambio aunque el usuario se elimine.
CREATE TABLE IF NOT EXISTS `auditoria_accesos` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `fecha` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `actor_id` int DEFAULT NULL,
  `actor_email` varchar(150) DEFAULT NULL,
  `accion` varchar(40) NOT NULL,
  `entidad` enum('rol','usuario','sistema') NOT NULL,
  `entidad_id` int DEFAULT NULL,
  `detalle` json NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_auditoria_fecha` (`fecha`),
  KEY `idx_auditoria_entidad` (`entidad`,`entidad_id`),
  KEY `idx_auditoria_actor` (`actor_id`),
  CONSTRAINT `fk_auditoria_actor` FOREIGN KEY (`actor_id`) REFERENCES `usuarios` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Permisos personalizados por usuario (overrides). Prioridad al evaluar:
-- override del usuario > permisos de sus roles > denegado.
CREATE TABLE IF NOT EXISTS `usuario_permisos` (
  `usuario_id` int NOT NULL,
  `permiso` varchar(60) NOT NULL,
  `concedido` tinyint(1) NOT NULL,
  `asignado_por` int DEFAULT NULL,
  `fecha_asignacion` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`usuario_id`,`permiso`),
  KEY `idx_usuario_permisos_permiso` (`permiso`),
  KEY `idx_usuario_permisos_asignado_por` (`asignado_por`),
  CONSTRAINT `fk_usuario_permisos_usuario` FOREIGN KEY (`usuario_id`) REFERENCES `usuarios` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_usuario_permisos_permiso` FOREIGN KEY (`permiso`) REFERENCES `permisos` (`nombre`) ON DELETE CASCADE ON UPDATE CASCADE,
  CONSTRAINT `fk_usuario_permisos_asignado_por` FOREIGN KEY (`asignado_por`) REFERENCES `usuarios` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Recomendaciones del panel aceptadas o descartadas. La clave describe la
-- situación (lote+nivel o producto+stock): si cambia, la recomendación reaparece.
CREATE TABLE IF NOT EXISTS `sugerencias_decisiones` (
  `clave` varchar(80) NOT NULL,
  `decision` enum('aceptada','descartada') NOT NULL,
  `usuario_id` int DEFAULT NULL,
  `fecha` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`clave`),
  KEY `idx_sugerencias_usuario` (`usuario_id`),
  CONSTRAINT `fk_sugerencias_usuario` FOREIGN KEY (`usuario_id`) REFERENCES `usuarios` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
