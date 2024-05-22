package main

import (
	"database/sql"
	"time"

	"grails/database"
	"grails/handlers"
	"grails/internals"
	"grails/models"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	// "golang.org/x/text/cases"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/template/slim/v2"
	"github.com/joho/godotenv"
)

var DB *gorm.DB

var (
	port = flag.String("port", ":5000", "Port to listen on")
	prod = flag.Bool("prod", false, "Enable prefork in Production")
)

func createDatabase(db *sql.DB, dbName string) error {
    // Create the database if it doesn't exist
    _, err := db.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", dbName))
    return err
}

type Field struct {
	Name string
	Type string
}

func capitalizeFirstLetter(str string) string {
	if len(str) == 0 {
		return str
	}
	return strings.ToUpper(string(str[0])) + str[1:]
}

// Converts a snake_case string to CamelCase.
func toCamelCase(str string) string {
	parts := strings.Split(str, "_")
	for i := range parts {
		parts[i] = capitalizeFirstLetter(parts[i])
	}
	return strings.Join(parts, "")
}

func toGoType(sqlType string) string {
	switch strings.ToUpper(sqlType) {
	case "VARCHAR(255)", "TEXT":
		return "string"
	case "INT":
		return "int"
	case "TIMESTAMP":
		return "time.Time"
	default:
		return "string"
	}
}

func generateCreateMigration(tableName string, fields []Field, reference ...string) {
	// Define the migration directory
	migrationDir := "migrations"
	modelDir := "models"

	// Ensure the migration directory exists
	err := os.MkdirAll(migrationDir, os.ModePerm)
	if err != nil {
		log.Fatalf("Failed to create migrations directory: %v", err)
	}

	// Generate the SQL for the migration
	migrationSQL := "-- +goose Up\n\n"
	migrationSQL += fmt.Sprintf("CREATE TABLE %ss (\n", tableName)
	migrationSQL += "  id INT AUTO_INCREMENT PRIMARY KEY,\n"
	for _, field := range fields {
		migrationSQL += fmt.Sprintf("  %s %s,\n", field.Name, field.Type)
	}
	if len(reference) > 0 {
		referenceTable := reference[0]
		migrationSQL += fmt.Sprintf("  %s INT NOT NULL,\n", referenceTable)
		migrationSQL += fmt.Sprintf("  FOREIGN KEY (%s) REFERENCES %s(id),\n", referenceTable, referenceTable)
	}
	migrationSQL += "  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP\n"
	migrationSQL += ");\n\n"

	migrationSQL += "-- +goose Down\n\n"
	migrationSQL += fmt.Sprintf("DROP TABLE %ss;", tableName)

	// Write the migration file
	time.Sleep(time.Millisecond * 1000)
	migrationFileName := fmt.Sprintf("%s/%s_create_%s_table.sql", migrationDir, time.Now().Format("20060102150405"), tableName)
	err = os.WriteFile(migrationFileName, []byte(migrationSQL), 0644)
	if err != nil {
		log.Fatalf("Failed to write migration file: %v", err)
	}

	fmt.Printf("Migration file %s created successfully.\n", migrationFileName)

	// Generate the Go model
	modelName := toCamelCase(tableName)
	modelContent := fmt.Sprintf("package models\n\nimport \"gorm.io/gorm\"\n\n// %s model\ntype %s struct {\n", modelName, modelName)
	modelContent += "gorm.Model\n"
	for _, field := range fields {
		fieldName := toCamelCase(field.Name)
		goType := toGoType(field.Type)
		modelContent += fmt.Sprintf("%s %s\n", fieldName, goType)
	}
	if len(reference) > 0 {
		referenceTable := reference[0]
		referenceField := toCamelCase(referenceTable)
		modelContent += fmt.Sprintf("%sID int\n", referenceField)
		modelContent += fmt.Sprintf("%s %s `gorm:\"foreignKey:%sID;references:ID\"`\n", referenceField, referenceField, referenceField)
	}
	modelContent += "}\n"

	// Write the model file.
	modelFileName := fmt.Sprintf("%s/%s.go", modelDir, tableName)
	err = os.WriteFile(modelFileName, []byte(modelContent), 0644)
	if err != nil {
		log.Fatalf("Failed to write model file: %v", err)
	}
	fmt.Printf("Model file %s created successfully.\n", modelFileName)

	// TODO: autoMigrate here gorm model.
}

func main() {
	
	err := godotenv.Load()
    if err != nil {
        log.Fatal("Error loading .env file")
    } else {
		fmt.Println("ENV Loaded.")
	}

	if len(os.Args) > 1 {
		// Check for migration command
		if os.Args[1] == "migrate" && len(os.Args) == 3 {
			internals.Migrate(os.Args[2]) // Run the migrate function with the direction
			return
		} else {
			log.Println("Usage: app migrate <up|down>")
			os.Exit(1)
		}
	}
    // Database connection string
    dsn := os.Getenv("DB_USER") + ":" + os.Getenv("DB_PASSWORD") + "@tcp(" + os.Getenv("DB_HOST") + ":" + os.Getenv("DB_PORT") + ")/"
    dbNot, err := sql.Open("mysql", dsn)
    if err != nil {
        log.Fatal(err)
    } else {
		fmt.Println("Database server connected.")
	}
    defer dbNot.Close()

    // Verify the connection
    if err := dbNot.Ping(); err != nil {
        log.Fatal(err)
    }

	// Create the
	err = createDatabase(dbNot, os.Getenv("DB_NAME"))
    if err != nil {
        log.Fatalf("Failed to create database: %v", err)
    } else {
		fmt.Println("Database connected.")
	}

	dsnWithDB := os.Getenv("DB_USER") + ":" + os.Getenv("DB_PASSWORD") + "@tcp(" + os.Getenv("DB_HOST") + ":" + os.Getenv("DB_PORT") + ")/" + os.Getenv("DB_NAME")
    db, err := sql.Open("mysql", dsnWithDB)
    if err != nil {
        log.Fatal(err)
    } else {
		fmt.Println("Database ready.")
	}
    defer db.Close()

	dbGorm, err := gorm.Open(mysql.Open(dsnWithDB), &gorm.Config{})
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	} else {
		fmt.Println("ORM Ready.")
		DB = dbGorm
	}


	// Create a new engine
	engine := slim.New("./views", ".slim")

	// Parse command-line flags
	flag.Parse()

	// Connected with database
	database.Connect()

	// Create fiber app
	app := fiber.New(fiber.Config{
		Prefork: *prod, // go run app.go -prod
		Views: engine,
	})

	// Middleware
	app.Use(recover.New())
	app.Use(logger.New())

	// Route to render the Slim template
	app.Get("/", func(c *fiber.Ctx) error {
		// Pass the title to the template
		return c.Render("index", fiber.Map{
			"Title": "Hello, Fiber with Slim!",
		})
	})

	// Create a /api/v1 endpoint
	v1 := app.Group("/api/v1")
	dbGorm.AutoMigrate(&models.Game{})
	dbGorm.AutoMigrate(&models.Player{})
	// Create a /game endpoint
	game := app.Group("/game")
	game.Get("/", handlers.GetGames(dbGorm))

	// Bind handlers
	v1.Get("/users", handlers.UserList)
	v1.Post("/users", handlers.UserCreate)

	// Setup static files
	app.Static("/js", "./static/public/js")
	app.Static("/img", "./static/public/img")

	// Handle not founds
	app.Use(handlers.NotFound)

	// tableName1 := "game"
	// fields1 := []Field{
	// 	{Name: "name", Type: "VARCHAR(100) NOT NULL"},
	// }

	// // Generate the migration files
	// generateCreateMigration(tableName1, fields1)

	// tableName2 := "player"
	// fields2 := []Field{
	// 	{Name: "name", Type: "VARCHAR(300) NOT NULL"},
	// }

	// // Generate the migration files
	// generateCreateMigration(tableName2, fields2, "game")

	// Listen on port 5000
	log.Fatal(app.Listen(*port)) 
}
