package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

func validarProducto(in productoEntrada) error {
	if in.Nombre == nil || strings.TrimSpace(*in.Nombre) == "" || utf8.RuneCountInString(*in.Nombre) > 150 || in.CategoriaID == nil || *in.CategoriaID <= 0 || (in.ProveedorID != nil && *in.ProveedorID <= 0) {
		return fmt.Errorf("nombre o categoría inválidos")
	}
	campos := []struct {
		v   *string
		max int
	}{{in.CodigoBarras, 50}, {in.Marca, 100}, {in.Presentacion, 80}, {in.AvisoComplemento, 255}, {in.ParaQueSirve, 16000}, {in.ModoToma, 16000}, {in.Contraindicaciones, 16000}, {in.Ingredientes, 16000}}
	for _, c := range campos {
		if c.v != nil && utf8.RuneCountInString(*c.v) > c.max {
			return fmt.Errorf("un texto supera su longitud máxima")
		}
	}
	if in.PrecioVenta != nil {
		if _, err := parsearDinero(strconv.FormatFloat(*in.PrecioVenta, 'f', -1, 64)); err != nil {
			return fmt.Errorf("precio_venta inválido; use hasta dos decimales")
		}
	}
	if in.MargenPorcentaje != nil && (math.IsNaN(*in.MargenPorcentaje) || *in.MargenPorcentaje < 0 || *in.MargenPorcentaje > 100) {
		return fmt.Errorf("margen_porcentaje debe estar entre 0 y 100")
	}
	if valueInt(in.StockMinimo) < 0 || valueInt(in.StockMinimo) > maxCantidadFEFO {
		return fmt.Errorf("stock_minimo fuera de rango")
	}
	temprana, liquidacion := valueIntDefault(in.DiasAlertaTemprana, 120), valueIntDefault(in.DiasAlertaLiquidacion, 60)
	if liquidacion < 0 || temprana < liquidacion || temprana > 3650 {
		return fmt.Errorf("los días de alerta deben cumplir 0 <= liquidación <= temprana <= 3650")
	}
	if len(in.Etiquetas) > 100 {
		return fmt.Errorf("demasiadas etiquetas")
	}
	vistos := map[int64]bool{}
	for _, id := range in.Etiquetas {
		if id <= 0 || vistos[id] {
			return fmt.Errorf("etiquetas debe contener ids positivos únicos")
		}
		vistos[id] = true
	}
	return nil
}
