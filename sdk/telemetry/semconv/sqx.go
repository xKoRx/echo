package semconv

import "go.opentelemetry.io/otel/attribute"

// SQX define atributos, features y eventos tipados para el dominio SQX
var SQX struct {
	// Atributos de negocio/técnicos
	Instrument    attribute.Key
	Strategy      attribute.Key
	Version       attribute.Key
	Wave          attribute.Key
	TaskFolder    attribute.Key
	SourceFolder  attribute.Key
	StoragePrefix attribute.Key
	WorkflowID    attribute.Key
	RunID         attribute.Key
	ActivityName  attribute.Key
	KeysCount     attribute.Key
	OutputCount   attribute.Key

	// Nuevos atributos
	TaskType           attribute.Key
	Host               attribute.Key
	HostKey            attribute.Key
	HostName           attribute.Key
	Direction          attribute.Key
	Timeframe          attribute.Key
	WorkerID           attribute.Key
	Step               attribute.Key
	DirectionTimeframe attribute.Key
	ErrorType          attribute.Key
	ConfigName         attribute.Key
	ConfigFolder       attribute.Key
	Buckets            attribute.Key

	// Features
	FeatureWatcher  attribute.Key
	FeatureWorker   attribute.Key
	FeatureWorkflow attribute.Key
	FeatureActivity attribute.Key

	// Eventos
	EventConfigLoaded    attribute.Key
	EventUploadCompleted attribute.Key
	EventWorkflowStarted attribute.Key
	EventActivityStarted attribute.Key
	EventResultsReady    attribute.Key
	EventUploadedResults attribute.Key
	EventDBRegistered    attribute.Key
	EventCompleted       attribute.Key
	EventError           attribute.Key
}

func init() {
	SQX.Instrument = attribute.Key("sqx.instrument")
	SQX.Strategy = attribute.Key("sqx.strategy")
	SQX.Version = attribute.Key("sqx.version")
	SQX.Wave = attribute.Key("sqx.wave")
	SQX.TaskFolder = attribute.Key("sqx.task_folder")
	SQX.SourceFolder = attribute.Key("sqx.source_folder")
	SQX.StoragePrefix = attribute.Key("sqx.storage_prefix")
	SQX.WorkflowID = attribute.Key("sqx.workflow_id")
	SQX.RunID = attribute.Key("sqx.run_id")
	SQX.ActivityName = attribute.Key("sqx.activity_name")
	SQX.KeysCount = attribute.Key("sqx.keys_count")
	SQX.OutputCount = attribute.Key("sqx.output_count")

	// Nuevos atributos
	SQX.TaskType = attribute.Key("sqx.task_type")
	SQX.Host = attribute.Key("sqx.host")
	SQX.HostKey = attribute.Key("sqx.host_key")
	SQX.HostName = attribute.Key("sqx.host_name")
	SQX.Direction = attribute.Key("sqx.direction")
	SQX.Timeframe = attribute.Key("sqx.timeframe")
	SQX.WorkerID = attribute.Key("sqx.worker_id")
	SQX.Step = attribute.Key("sqx.step")
	SQX.DirectionTimeframe = attribute.Key("sqx.direction_timeframe")
	SQX.ErrorType = attribute.Key("error_type")
	SQX.ConfigName = attribute.Key("sqx.config_name")
	SQX.ConfigFolder = attribute.Key("sqx.config_folder")
	SQX.Buckets = attribute.Key("sqx.buckets")

	SQX.FeatureWatcher = attribute.Key("SQXWatcher")
	SQX.FeatureWorker = attribute.Key("SQXWorker")
	SQX.FeatureWorkflow = attribute.Key("SQXWorkflow")
	SQX.FeatureActivity = attribute.Key("SQXActivity")

	SQX.EventConfigLoaded = attribute.Key("config_loaded")
	SQX.EventUploadCompleted = attribute.Key("upload_completed")
	SQX.EventWorkflowStarted = attribute.Key("workflow_started")
	SQX.EventActivityStarted = attribute.Key("activity_started")
	SQX.EventResultsReady = attribute.Key("results_ready")
	SQX.EventUploadedResults = attribute.Key("uploaded_results")
	SQX.EventDBRegistered = attribute.Key("db_registered")
	SQX.EventCompleted = attribute.Key("completed")
	SQX.EventError = attribute.Key("error")
}
