package repo

import (
	"database/sql"
	"fmt"
	"time"

	"context"
	"errors"
	"strconv"

	log "github.com/Sainarasimhan/Logger"
	svcerr "github.com/Sainarasimhan/go-error/err"
	"go.opentelemetry.io/otel/api/kv"
	"go.opentelemetry.io/otel/api/metric"
	"go.opentelemetry.io/otel/api/trace"

	//Postgres Driver
	_ "github.com/lib/pq"
)

const (
	// INSERT - const for db stmts
	INSERT = "Insert"
	// UPDATE - const for db stmts
	UPDATE = "Update"
	// DELETE - const for db stmts
	DELETE = "Delete"
	// LIST - const for db stmts
	LIST = "List"
)

// Repository - Interface for DB operations
type Repository interface {
	Insert(context.Context, Request) (Response, error)
	Update(context.Context, Request) error
	List(context.Context, Request) ([]Details, error)
	Delete(context.Context, Request) error
	Close()
}

// Request - Create Request
type Request struct {
	ID     int
	Param1 string
	Param2 string
	Param3 string
}

// Response - Create response
type Response struct {
	ID int
}

// Details - List response
type Details struct {
	ID     int
	Param1 string
	Param2 string
	Param3 string
}

// Metrics - structure to hold db Metrics
type Metrics struct {
	OpenCnx    metric.Int64ValueObserver
	IdleCnx    metric.Int64ValueObserver
	IdelClosed metric.Int64SumObserver
	db         *sql.DB
	*log.Logger
}

//PostgresDB - Implementation of Repository with Postgres DB
type PostgresDB struct {
	*log.Logger
	db       *sql.DB
	maxConns int
	st       stmts
	ot       trace.Tracer
}

type stmts map[string]stmt

type stmt struct {
	query string
	stmt  *sql.Stmt
}

// PostgresOption  Options for postgres DB
type PostgresOption func(*PostgresDB)

//DB Statements
var dbstmts = stmts{
	INSERT: {"Insert into SampleTable(ID,Param1,Param2,Param3) values ($1,$2,$3,$4);", nil},
	DELETE: {"Delete from SampleTable where ID = $1;", nil},
	UPDATE: {"Update SampleTable set Param1 = $1, Param2 =$2, Param3 =$3 where ID = $4;", nil},
	LIST:   {"select * from SampleTable where ID = $1 limit 10;", nil},
}

func (s *stmts) isValid() (err error) {
	for op, st := range *s {
		if st.stmt == nil {
			return fmt.Errorf("DB Statment for %s Not available", op)
		}
	}
	return
}

// SetMaxPostgresConn -- Max Postgres Connections
func SetMaxPostgresConn(maxConns int) PostgresOption {
	return func(p *PostgresDB) {
		p.maxConns = maxConns
	}
}

// SetTracer - Sets up Opentelemetry tracer
func SetTracer(ot trace.Tracer) PostgresOption {
	return func(p *PostgresDB) {
		p.ot = ot
	}
}

// EnableMetrics - Enables openmetrics observer to get DB Metrics
func EnableMetrics(m *Metrics) PostgresOption {
	return func(p *PostgresDB) {
		m.db = p.db
		m.Logger = p.Logger
	}
}

//NewPostgres - Creates new instance which implementes Repository
func NewPostgres(connStr string, lg *log.Logger, options ...PostgresOption) (repo Repository, err error) {
	connStr = connStr + " connect_timeout=5"
	lg.Debug("Action", "NewDB", "connStr", connStr)("New DB requested")
	p := &PostgresDB{
		Logger:   lg,
		maxConns: 5, //Default max conns
	}

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		lg.Error("Action", "connect", "db", "postgres")(err.Error())
		return nil, svcerr.Wrap("Error opening Database", err)
	}
	p.db = db

	for _, option := range options {
		option(p)
	}

	p.db.SetMaxIdleConns(p.maxConns)
	p.db.SetMaxOpenConns(p.maxConns)
	p.db.SetConnMaxLifetime(1 * time.Hour)

	//Create Table if not exists already
	p.createTable() //Ignore any error

	if err := p.prepareStmts(); err != nil {
		lg.Error("Action", "PrepareStmts")("Failred preparing Stmts")
		//Ignore any error
	}

	lg.Info("Action", "NewDB")("New DB service created")
	repo = CommonMiddleware(lg, p.ot)(p) //Wrap repo with Error Middelware

	return
}

// prepare statments and uses during actual calls
func (p *PostgresDB) prepareStmts() (err error) {
	if p.db == nil {
		p.Error("Action", "PrepareStmt")("DB Connection Not available")
		return errors.New("DB Connection Not Available")
	}

	for op, st := range dbstmts {
		if st.stmt, err = p.db.Prepare(st.query); err != nil {
			p.Error("Action", "PrepareStmt", "stmt", op)(err.Error())
			return
		}
		dbstmts[op] = st
		p.Debug("stmt", op)("Statement Prepared")
	}
	p.Info("Action", "PrepareStmt")("All Statement Prepared Successfully")
	p.st = dbstmts

	return nil
}

// Function to create Table, can be used for initial setup
func (p *PostgresDB) createTable() (err error) {

	if _, err = p.db.Exec(`CREATE TABLE SampleTable (ID int,
		Param1 varchar(50), 
		Param2 varchar(100),  
		Param3 varchar(100));`); err != nil {
		//Ignore the error, but log it
		p.Error("Stmt", "Create")(err.Error())
		return
	}
	p.Info("Stmt", "Create")("Created Table")
	return
}

// Verifies if DB connection and stmts are setup.
func (p *PostgresDB) validateDBConn() error {
	if p.db == nil {
		return svcerr.InternalErr("Database Not Initialized")
	}

	/*if err := p.db.Ping(); err != nil {
		p.Error("action", "ping")(err.Error())
		return errors.New("DB connection Error/UnAvailable")
	}*/ //TODO - Ping needed?

	if p.st != nil {
		if err := p.st.isValid(); err != nil {
			p.Error("Action", "ping")(err.Error())
			return svcerr.Wrap("DB Statments Error", err)
		}
	} else {
		if err := p.prepareStmts(); err != nil {
			p.Error("Action", "ping-PrepareStmts")(err.Error())
			return errors.New("DB Statements Error/UnAvailable")
		}
	}

	p.Debug("Action", "Ping", "DB status", "OK")
	return nil
}

// function to execute statements
func (p *PostgresDB) genericStmtExec(ctx context.Context, op string, args ...interface{}) (err error) {

	if err = p.validateDBConn(); err != nil {
		return
	}

	res, err := p.st[op].stmt.ExecContext(ctx, args...)
	if err != nil {
		p.Error("req", fmt.Sprintf("%+v", args), "Action", op)(err.Error())
		err = svcerr.Wrap("DB "+op+" Failure", err)
		return
	}

	ra, _ := res.RowsAffected()
	p.Info("req", fmt.Sprintf("%+v", args), "RowsAffected", strconv.Itoa(int(ra)), "Action", op)("Successful operation")

	return
}

// Interface Implementations

// Insert -- Inserts new entry
func (p *PostgresDB) Insert(ctx context.Context, r Request) (resp Response, err error) {
	// No response returned atm, can add return params as needed
	return Response{}, p.genericStmtExec(ctx, INSERT, r.ID, r.Param1, r.Param2, r.Param3)
}

// Update - Update entries with new values
func (p *PostgresDB) Update(ctx context.Context, r Request) (err error) {
	return p.genericStmtExec(ctx, UPDATE, r.ID, r.Param1, r.Param2, r.Param3)
}

// Delete - Deletes entry
func (p *PostgresDB) Delete(ctx context.Context, r Request) (err error) {
	return p.genericStmtExec(ctx, DELETE, r.ID)
}

// List - returns the entries matching provided ID
func (p *PostgresDB) List(ctx context.Context, r Request) (list []Details, err error) {
	if err = p.validateDBConn(); err != nil {
		return
	}

	idStr := strconv.Itoa(r.ID)
	rows, err := p.st[LIST].stmt.QueryContext(ctx, r.ID)
	if err != nil {
		p.Error("req", idStr, "Action", LIST)(err.Error())
		err = svcerr.Wrap("DB Select Failure", err)
		return
	}

	for rows.Next() {
		d := Details{}
		if err = rows.Scan(&d.ID, &d.Param1, &d.Param2, &d.Param3); err != nil {
			p.Error("req", idStr, "Action", LIST)("Error Scanning rows %s", err.Error())
			return
		}
		list = append(list, d)
	}
	rows.Close()

	p.Info("req", idStr, "Action", LIST)("Retrieved list of (%d) rows", len(list))
	return list, err
}

// Close -- Close all db and stmts
func (p *PostgresDB) Close() {
	//Close all stmts
	for _, st := range dbstmts {
		if st.stmt != nil {
			st.stmt.Close()
		}
	}
	if p.db != nil {
		p.db.Close()
	}
}

// DoMetrics - Collects metrics as part of opentelemtry batch observer
func (m *Metrics) DoMetrics() metric.BatchObserverCallback {
	return func(ctx context.Context, result metric.BatchObserverResult) {
		if m.db == nil || m.Logger == nil {
			return
		}
		stats := m.db.Stats()
		result.Observe(
			[]kv.KeyValue{
				kv.String("name", "DB Metrics"),
			},
			m.OpenCnx.Observation(int64(stats.OpenConnections)),
			m.IdelClosed.Observation(int64(stats.MaxIdleClosed)),
			m.IdleCnx.Observation(int64(stats.Idle)),
		)
		m.Debug("DB", "Metrics")("%+v", stats)
	}
}
