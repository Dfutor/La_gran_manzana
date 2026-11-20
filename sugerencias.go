package main

// Recomendaciones del panel principal («sugerencias» en el API, para no
// confundirlas con /api/recomendaciones, que recomienda productos por necesidad).
//
// Se derivan en cada consulta de las alertas de vencimiento y de stock bajo.
// Cada una tiene una clave que describe la situación (lote + nivel, producto +
// stock); aceptar o descartar se guarda en sugerencias_decisiones para todo el
// equipo. Si la situación cambia (el lote pasa de «liquidar» a «vencido», el
// stock vuelve a bajar), la clave cambia y la recomendación reaparece.

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

type sugerencia struct {
	Clave      string  `json:"clave"`
	Tipo       string  `json:"tipo"`      // vencido | liquidar | devolver | destacar | reponer
	Prioridad  int     `json:"prioridad"` // 1 alta, 2 media, 3 baja
	Titulo     string  `json:"titulo"`
	Motivo     string  `json:"motivo"`
	Accion     string  `json:"accion"`
	Destino    string  `json:"destino"` // baja | lote | producto: a dónde lleva «Aceptar»
	ProductoID int64   `json:"producto_id"`
	LoteID     int64   `json:"lote_id,omitempty"`
	Cantidad   int64   `json:"cantidad"`
	Valor      float64 `json:"valor_en_riesgo"`
}

func plural(n int64, uno, varios string) string {
	if n == 1 || n == -1 {
		return fmt.Sprintf("%d %s", n, uno)
	}
	return fmt.Sprintf("%d %s", n, varios)
}

// construirSugerencias es pura: ordena por prioridad y luego por valor en riesgo.
func construirSugerencias(vencimientos []alertaVencimiento, stock []alertaStock) []sugerencia {
	lista := []sugerencia{}
	for _, v := range vencimientos {
		s := sugerencia{
			Clave: fmt.Sprintf("lote:%d:%s", v.LoteID, v.Nivel), Tipo: v.Nivel, Accion: v.AccionSugerida,
			ProductoID: v.ProductoID, LoteID: v.LoteID, Cantidad: v.CantidadActual, Valor: v.ValorEnRiesgo,
		}
		if s.Accion == "" {
			s.Accion = sugerirAccion(v.Nivel, v.AceptaDevolucion != nil && *v.AceptaDevolucion)
		}
		lote := fmt.Sprintf("lote #%d", v.LoteID)
		if v.CodigoLote != nil && strings.TrimSpace(*v.CodigoLote) != "" {
			lote = "lote " + *v.CodigoLote
		}
		unidades := plural(v.CantidadActual, "unidad", "unidades")
		switch v.Nivel {
		case "vencido":
			s.Prioridad, s.Destino = 1, "baja"
			s.Titulo = "Retirar " + v.Producto + " vencido"
			s.Motivo = fmt.Sprintf("El %s venció hace %s y aún tiene %s en el estante.", lote, plural(-v.DiasRestantes, "día", "días"), unidades)
		case "liquidar":
			s.Prioridad, s.Destino = 1, "producto"
			s.Titulo = "Liquidar " + v.Producto
			s.Motivo = fmt.Sprintf("El %s vence en %s con %s por vender.", lote, plural(v.DiasRestantes, "día", "días"), unidades)
			if v.DiasRestantes == 0 {
				s.Motivo = fmt.Sprintf("El %s vence hoy con %s por vender.", lote, unidades)
			}
		default: // devolver
			s.Prioridad = 3
			if v.AceptaDevolucion != nil && *v.AceptaDevolucion {
				proveedor := "el proveedor"
				if v.Proveedor != nil {
					proveedor = *v.Proveedor
				}
				s.Tipo, s.Destino = "devolver", "baja"
				s.Titulo = "Devolver " + v.Producto + " a " + proveedor
			} else {
				s.Tipo, s.Destino = "destacar", "producto"
				s.Titulo = "Destacar " + v.Producto + " en el mostrador"
			}
			s.Motivo = fmt.Sprintf("El %s vence en %s (%s); todavía hay margen para actuar.", lote, plural(v.DiasRestantes, "día", "días"), unidades)
		}
		lista = append(lista, s)
	}
	for _, p := range stock {
		s := sugerencia{
			Clave: fmt.Sprintf("stock:%d:%d", p.ID, p.Stock), Tipo: "reponer", Prioridad: 2, Destino: "lote",
			ProductoID: p.ID, Cantidad: p.FaltanteParaMinimo, Titulo: "Reponer " + p.Nombre,
			Accion: "Pedir al proveedor y registrar el ingreso del lote",
		}
		if p.Stock <= 0 {
			s.Prioridad = 1
			s.Motivo = fmt.Sprintf("Está agotado; el mínimo es %s.", plural(p.StockMinimo, "unidad", "unidades"))
		} else {
			s.Motivo = fmt.Sprintf("Quedan %s vigentes y el mínimo es %d.", plural(p.Stock, "unidad", "unidades"), p.StockMinimo)
		}
		lista = append(lista, s)
	}
	sort.SliceStable(lista, func(i, j int) bool {
		if lista[i].Prioridad != lista[j].Prioridad {
			return lista[i].Prioridad < lista[j].Prioridad
		}
		return lista[i].Valor > lista[j].Valor
	})
	return lista
}

func (a *app) sugerenciasVigentes(r *http.Request) ([]sugerencia, error) {
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	venc, err := a.consultarVencimientos(ctx)
	if err != nil {
		return nil, err
	}
	stock, err := a.consultarStockBajo(ctx)
	if err != nil {
		return nil, err
	}
	return construirSugerencias(venc, stock), nil
}

// GET /api/sugerencias: recomendaciones vigentes que nadie ha aceptado ni descartado.
func (a *app) listarSugerencias(w http.ResponseWriter, r *http.Request) {
	todas, err := a.sugerenciasVigentes(r)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	pendientes := []sugerencia{}
	if len(todas) > 0 {
		ctx, cancel := ctxConsulta(r)
		defer cancel()
		claves := make([]any, len(todas))
		for i, s := range todas {
			claves[i] = s.Clave
		}
		decididas, err := leerTextos(a.db.QueryContext(ctx, `SELECT clave FROM sugerencias_decisiones WHERE clave IN (`+placeholders(len(claves))+`)`, claves...))
		if err != nil {
			errorInterno(w, r, err)
			return
		}
		ya := conjunto(decididas)
		for _, s := range todas {
			if !ya[s.Clave] {
				pendientes = append(pendientes, s)
			}
		}
	}
	escribirJSON(w, 200, pendientes)
}

// POST /api/sugerencias/decision {clave, decision: aceptada|descartada}
func (a *app) decidirSugerencia(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Clave    string `json:"clave"`
		Decision string `json:"decision"`
	}
	if !decodificarJSON(w, r, &in) {
		return
	}
	if in.Decision != "aceptada" && in.Decision != "descartada" {
		escribirError(w, 400, "decision debe ser aceptada o descartada")
		return
	}
	if in.Clave == "" || len(in.Clave) > 80 {
		escribirError(w, 400, "clave inválida")
		return
	}
	// Solo se decide sobre recomendaciones que existen ahora: evita guardar claves arbitrarias.
	todas, err := a.sugerenciasVigentes(r)
	if err != nil {
		errorInterno(w, r, err)
		return
	}
	var elegida *sugerencia
	for i := range todas {
		if todas[i].Clave == in.Clave {
			elegida = &todas[i]
			break
		}
	}
	if elegida == nil {
		escribirError(w, 404, "la recomendación ya no está vigente; actualice el panel")
		return
	}
	ctx, cancel := ctxConsulta(r)
	defer cancel()
	actor := usuarioActual(r)
	var previa string
	err = a.db.QueryRowContext(ctx, `SELECT decision FROM sugerencias_decisiones WHERE clave=?`, in.Clave).Scan(&previa)
	if err == nil {
		escribirError(w, 409, "otra persona ya marcó esta recomendación como "+previa)
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		errorInterno(w, r, err)
		return
	}
	if _, err = a.db.ExecContext(ctx, `INSERT INTO sugerencias_decisiones(clave,decision,usuario_id) VALUES(?,?,?)`, in.Clave, in.Decision, actor.ID); err != nil {
		responderFallo(w, r, err, "otra persona ya decidió esta recomendación", "")
		return
	}
	escribirJSON(w, 200, map[string]any{"decision": in.Decision, "sugerencia": elegida})
}
