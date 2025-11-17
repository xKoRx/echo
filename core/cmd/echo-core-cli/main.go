package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/xKoRx/echo/core/internal"
	"github.com/xKoRx/echo/sdk/domain/handshake"
	"google.golang.org/protobuf/encoding/protojson"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	switch command {
	case "handshake":
		runHandshake(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "comando desconocido: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	usage := `echo-core-cli - herramientas operativas para Echo Core

Uso:
  echo-core-cli handshake evaluate --account <id> [--timeout 15s] [--no-send] [--json]

Comandos:
  handshake evaluate   Fuerza la re-evaluación de handshake para una cuenta.
`
	fmt.Fprintln(os.Stderr, usage)
}

func runHandshake(args []string) {
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	subcommand := args[0]
	switch subcommand {
	case "evaluate":
		handshakeEvaluate(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "subcomando handshake desconocido: %s\n", subcommand)
		printUsage()
		os.Exit(1)
	}
}

func handshakeEvaluate(args []string) {
	fs := flag.NewFlagSet("handshake evaluate", flag.ExitOnError)
	accountID := fs.String("account", "", "Cuenta (account_id) que se desea evaluar")
	timeout := fs.Duration("timeout", 15*time.Second, "Timeout para la evaluación")
	send := fs.Bool("send", true, "Reenviar el resultado al Agent")
	jsonOutput := fs.Bool("json", false, "Imprimir el resultado en formato JSON")
	noSend := fs.Bool("no-send", false, "No reenviar el resultado al Agent")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "error parseando flags: %v\n", err)
		os.Exit(1)
	}

	if *accountID == "" {
		fmt.Fprintln(os.Stderr, "--account es requerido")
		fs.Usage()
		os.Exit(1)
	}

	sendResult := *send && !*noSend
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	core, err := internal.New(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error inicializando core: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if shutdownErr := core.Shutdown(); shutdownErr != nil {
			fmt.Fprintf(os.Stderr, "error cerrando core: %v\n", shutdownErr)
		}
	}()

	evaluation, err := core.EvaluateHandshakeForAccount(ctx, *accountID, sendResult)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error evaluando handshake: %v\n", err)
		os.Exit(1)
	}

	if evaluation == nil {
		fmt.Printf("No existe evaluación previa para la cuenta %s\n", *accountID)
		return
	}

	if *jsonOutput {
		printEvaluationJSON(evaluation)
		return
	}

	printEvaluationText(evaluation)
}

func printEvaluationJSON(evaluation *handshake.Evaluation) {
	result := evaluation.ToProtoResult()
	options := protojson.MarshalOptions{Indent: "  ", EmitUnpopulated: true}
	data, err := options.Marshal(result)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error serializando resultado: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

func printEvaluationText(evaluation *handshake.Evaluation) {
	lines := []string{
		fmt.Sprintf("Cuenta: %s (%s)", evaluation.AccountID, evaluation.PipeRole),
		fmt.Sprintf("Estado: %s", strings.ToUpper(evaluation.Status.ToProto().String())),
		fmt.Sprintf("ProtocolVersion: %d", evaluation.ProtocolVersion),
		fmt.Sprintf("ClientSemver: %s", evaluation.ClientSemver),
		fmt.Sprintf("EvaluationID: %s", evaluation.EvaluationID),
	}
	if len(evaluation.Errors) > 0 {
		lines = append(lines, "Errores globales:")
		for _, issue := range evaluation.Errors {
			lines = append(lines, fmt.Sprintf("  - %s: %s", issue.Code.String(), issue.Message))
		}
	}
	if len(evaluation.Warnings) > 0 {
		lines = append(lines, "Warnings globales:")
		for _, issue := range evaluation.Warnings {
			lines = append(lines, fmt.Sprintf("  - %s: %s", issue.Code.String(), issue.Message))
		}
	}
	if len(evaluation.Entries) > 0 {
		lines = append(lines, "Símbolos:")
		for _, entry := range evaluation.Entries {
			lines = append(lines, fmt.Sprintf("  * %s (%s) → %s", entry.CanonicalSymbol, entry.BrokerSymbol, strings.ToUpper(entry.Status.ToProto().String())))
		}
	}

	fmt.Println(strings.Join(lines, "\n"))
}
