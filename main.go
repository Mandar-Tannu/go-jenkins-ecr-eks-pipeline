package main

import(
	"context"
	"database/sql"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	_"github.com/lib/pq"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

/* DATABASE CONNECTION */
var rdsDB *sql.DB
var instanceID string
var awsCfg aws.Config

func getEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		log.Fatalf("level=FATAL service=go-app error=missing_env_var key=%s", key)
	}
	return val
}

func connectDB(prefix string) *sql.DB {
	dsn := "host=" + getEnv(prefix+"_HOST") +
	" port=" + getEnv(prefix+"_PORT") +
	" user=" + getEnv(prefix+"_USER") +
	" password=" + getEnv(prefix+"_PASSWORD") +
	" dbname=" + getEnv(prefix+"_NAME") +
	" sslmode=" + getEnv(prefix+"_SSLMODE")


	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("level=FATAL service=go-app error=db_open_failed db=%s err=%v", prefix, err)
	}

	if err := db.Ping(); err != nil {
		log.Fatalf("level=FATAL service=go-app error=db_ping_failed db=%s err=%v", prefix, err)
	}

	log.Printf("level=INFO service=go-app event=db_connected db=%s instance=%s", prefix, instanceID)
	return db
}

func initDatabase() {
	rdsDB = connectDB("RDS_DB")
	createTable(rdsDB)
}

func createTable(db *sql.DB){
	query := `
	CREATE TABLE IF NOT EXISTS users(
		id SERIAL PRIMARY KEY,
		name TEXT NOT NULL,
		email TEXT NOT NULL,
		phone TEXT NOT NULL,
		document_bucket TEXT NOT NULL,
		document_key TEXT NOT NULL,
		kyc_status TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)
	`

	if _, err := db.Exec(query); err != nil {
		log.Fatalf("level=FATAL service=go-app error=create_table_failed err=%v", err)
	}

	log.Printf("level=INFO service=go-app event=table_ready table=users instance=%s", instanceID)
}

/* HTTP HANDLERS */
func healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Optional: check DB connectivity
	if err := rdsDB.Ping(); err != nil {
		http.Error(w, "Database connection failed", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func readinessHandler(w http.ResponseWriter, r *http.Request) {
	if err := rdsDB.Ping(); err != nil {
		http.Error(w, "Not ready", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func formHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		log.Printf("level=WARN service=go-app event=invalid_method path=/ method=%s instance=%s", r.Method, instanceID)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	log.Printf("level=INFO service=go-app event=serve_form path=/ instance=%s", instanceID)
	http.ServeFile(w, r, "index.html")
}

func submitHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		log.Printf("level=WARN service=go-app event=invalid_method path=/submit method=%s instance=%s", r.Method, instanceID)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("kyc_document")
	if err != nil {
		http.Error(w, "Failed to read KYC document", http.StatusBadRequest)
		return
	}
	defer file.Close()

	bucket, key, err := uploadToS3(file, header.Filename)
	if err != nil {
    	log.Printf("level=ERROR service=go-app event=s3_upload_failed err=%v instance=%s", err, instanceID)
    	http.Error(w, "Failed to upload document to S3", http.StatusInternalServerError)
    	return
	}

	name := r.FormValue("name")
	email := r.FormValue("email")
	phone := r.FormValue("phone")

	query := `
	INSERT INTO users(name, email, phone, document_bucket, document_key, kyc_status)
	VALUES ($1, $2, $3, $4, $5, $6)
	`

	if _, err := rdsDB.Exec(query, name, email, phone, bucket, key, "KYC_UPLOADED"); err != nil {
		log.Printf("level=ERROR service=go-app event=db_insert_failed name=%s email=%s phone=%s err=%v instance=%s", name, email, phone, err, instanceID)
		http.Error(w, "Failed to store data in RDS", http.StatusInternalServerError)
		return
	}

	log.Printf("level=INFO service=go-app event=user_created name=%s email=%s phone=%s instance=%s", name, email, phone, instanceID)
	w.Write([]byte("User data stored by instance: "+instanceID))
}

func uploadToS3(file multipart.File, filename string) (string, string, error) {
	bucket := getEnv("S3_BUCKET_NAME")

	client := s3.NewFromConfig(awsCfg)

	key := "kyc-docs/" + time.Now().Format("20060102-150405") + "-" + filepath.Base(filename)

	_, err := client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key: aws.String(key),
		Body: file,
	})

	if err != nil {
		return "", "", err
	}

	return bucket, key, nil
}

/* MAIN */
func main() {
	// log format: timestamp + file:line
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	host, err := os.Hostname()
	if err != nil {
		instanceID = "unknown-instance"
	} else {
		instanceID = host
	}

	log.Printf("level=INFO service=go-app event=app_start instance=%s", instanceID)

	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion("ap-south-1"))
	if err != nil {
		log.Fatalf("AWS config failed: %v", err)
	}
	awsCfg = cfg

	initDatabase()

	http.HandleFunc("/", formHandler)
	http.HandleFunc("/submit", submitHandler)
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/ready", readinessHandler)

	log.Printf("level=INFO service=go-app event=server_started port=8080 instance=%s", instanceID)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
