package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// cargarEnv lee un archivo KEY=VALOR y exporta las variables que aún no
// estén definidas en el entorno (las del entorno tienen prioridad).
// Si el archivo no existe no es un error: se usan solo las del entorno.
func cargarEnv(ruta string) error {
	f, err := os.Open(ruta)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		linea := strings.TrimSpace(sc.Text())
		if linea == "" || strings.HasPrefix(linea, "#") {
			continue
		}
		clave, valor, ok := strings.Cut(linea, "=")
		if !ok {
			return fmt.Errorf("%s:%d: se esperaba CLAVE=VALOR", ruta, n)
		}
		clave = strings.TrimSpace(clave)
		valor = strings.Trim(strings.TrimSpace(valor), `"'`)
		if _, existe := os.LookupEnv(clave); !existe {
			os.Setenv(clave, valor)
		}
	}
	return sc.Err()
}

// config agrupa la configuración que viene de .env / entorno.
type config struct {
	DBUser, DBPass, DBHost, DBName string
	Addr                           string
}

func leerConfig() (config, error) {
	c := config{
		DBUser: os.Getenv("DB_USER"),
		DBPass: os.Getenv("DB_PASS"),
		DBHost: os.Getenv("DB_HOST"),
		DBName: os.Getenv("DB_NAME"),
		Addr:   os.Getenv("APP_ADDR"),
	}
	if c.DBHost == "" {
		c.DBHost = "127.0.0.1:3306"
	}
	if c.Addr == "" {
		c.Addr = "127.0.0.1:8080"
	}
	var faltan []string
	if c.DBUser == "" {
		faltan = append(faltan, "DB_USER")
	}
	if c.DBPass == "" {
		faltan = append(faltan, "DB_PASS")
	}
	if c.DBName == "" {
		faltan = append(faltan, "DB_NAME")
	}
	if len(faltan) > 0 {
		return c, fmt.Errorf("faltan variables en .env: %s", strings.Join(faltan, ", "))
	}
	return c, nil
}

// abrirDB crea el pool de conexiones. No arma el DSN a mano: usa mysql.NewConfig.
func abrirDB(c config) (*sql.DB, error) {
	mc := mysql.NewConfig()
	mc.User = c.DBUser
	mc.Passwd = c.DBPass
	mc.Net = "tcp"
	mc.Addr = c.DBHost
	mc.DBName = c.DBName
	mc.ParseTime = true
	mc.Loc = time.Local
	mc.Timeout = 5 * time.Second
	mc.ReadTimeout = 30 * time.Second
	mc.WriteTimeout = 30 * time.Second

	conector, err := mysql.NewConnector(mc)
	if err != nil {
		return nil, err
	}
	db := sql.OpenDB(conector)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)
	return db, nil
}
