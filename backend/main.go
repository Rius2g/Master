package main


import (
    "fmt"
    "log"

   "github.com/joho/godotenv"
)


//this is the package so we can import these functions in other projects
func main() {
    if err := godotenv.Load(); err != nil {
        log.Fatal("Error loading .env file") 
    }
    fmt.Println("Starting the application...")
   fmt.Println("ABI loaded successfully")

}


