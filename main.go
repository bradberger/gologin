package main

import (
    "fmt"
    "io"
    "log"
    "log/syslog"
    "flag"
    "math/rand"
    "os"
    "strings"
    "sync"

    "github.com/jinzhu/gorm"
    _ "github.com/jinzhu/gorm/dialects/sqlite"

    "github.com/gin-gonic/gin"
    "github.com/gin-gonic/contrib/sessions"
    "github.com/gin-gonic/contrib/expvar"

    "github.com/markbates/goth"
    "github.com/markbates/goth/providers/gplus"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

var (
    userEmails    string
    adminEmails   string
    sessionSecret string
    publicAddr    string
    listen        string
    gAuthSecret   string
    gAuthKey      string
    dbName        string
    db            *gorm.DB
    store         sessions.CookieStore
)

func init() {

    // Log to syslog and stdout
    log.SetOutput(os.Stderr)
	if logWriter, err := syslog.New(syslog.LOG_NOTICE, "screvle-vm"); err == nil {
		log.SetOutput(io.MultiWriter(os.Stderr, logWriter))
	}

    // Set up flags
    flag.StringVar(&userEmails, "user", os.Getenv("USER_EMAIL"), "A comma separated list of regular user emails")
    flag.StringVar(&adminEmails, "admin", os.Getenv("ADMIN_EMAIL"), "A comma separated list of admin user emails")
    flag.StringVar(&sessionSecret, "session-secret", os.Getenv("SESSION_SECRET"), "The session secret encryption hash")
    flag.StringVar(&publicAddr, "public-addr", os.Getenv("PUBLIC_ADDR"), "The external hostname to use for OAuth callbacks")
    flag.StringVar(&listen, "listen", os.Getenv("LISTEN"), "The address for HTTP server to listen on")
    flag.StringVar(&gAuthSecret, "gauth-secret", os.Getenv("GPLUS_SECRET"), "The Google OAuth secret")
    flag.StringVar(&gAuthKey, "gauth-key", os.Getenv("GPLUS_KEY"), "The Google OAuth key")
    flag.StringVar(&dbName, "database", os.Getenv("SQLITE_DB_FILE"), "The SQLite3 database file to use")
    flag.Parse()

    // Generate a psuedo-random session secret for testing.
    if sessionSecret == "" {
        sessionSecret = func() string {
            b := make([]byte, 32)
            for i := range b {
                b[i] = letterBytes[rand.Int63() % int64(len(letterBytes))]
            }
            return string(b)
        }()
    }

    if listen == "" {
        listen = ":8080"
    }

    var wg sync.WaitGroup

    // Init the database
    wg.Add(1)
    go func() {

        defer wg.Done()
        var err error
        if dbName == "" {
            log.Fatalf("No database file supplied")
        }
        db, err = gorm.Open("sqlite3", dbName)
        if err != nil {
            log.Fatalf("Could not open database: %v", err)
        }

        // Init the database.
        db.AutoMigrate(&User{}, &Data{})

        // Create some test data
        wg.Add(1)
        go func() {
            defer wg.Done()
            db.FirstOrCreate(&Data{}, Data{Type: RegularUser, Key: "user",  Value: "foo"})
            db.FirstOrCreate(&Data{}, Data{Type: AdminUser, Key: "admin", Value: "bar"})
        }()

        // Now create users based on the flags.
        for _, e := range strings.Split(userEmails, ",") {
            wg.Add(1)
            go func(e string) {
                defer wg.Done()
                u := new(User)
                db.FirstOrCreate(u, User{Email: strings.TrimSpace(e)})
                u.Type = RegularUser
                if err := db.Save(&u).Error; err != nil {
                    log.Printf("[ERROR] Didn't save user: %v", err)
                    return
                }
                log.Printf("[DEBUG] Added user with email: %s", e)
            }(e)
        }

        for _, e := range strings.Split(adminEmails, ",") {
            wg.Add(1)
            go func(e string) {
                defer wg.Done()
                u := new(User)
                db.FirstOrCreate(u, User{Email: strings.TrimSpace(e)}).Update("type", AdminUser)
                u.Type = AdminUser
                if err := db.Save(&u).Error; err != nil {
                    log.Printf("[ERROR] Didn't save user: %v", err)
                    return
                }
                log.Printf("[DEBUG] Added admin user with email: %s", e)
            }(e)
        }

    }()

    // Setup OAuth
    wg.Add(1)
    go func() {
        defer wg.Done()
        if publicAddr == "" {
            log.Fatalf("No OAuth callback address supplied")
        }

        if gAuthSecret == "" {
            log.Fatalf("No Google OAuth secret supplied")
        }

        if gAuthKey == "" {
            log.Fatalf("No Google OAuth key supplied")
        }
        goth.UseProviders(
            gplus.New(gAuthKey, gAuthSecret, fmt.Sprintf("%s/auth/gplus/callback", publicAddr)),
        )
    }()

    // Setup sessions
    wg.Add(1)
    go func() {
        defer wg.Done()
        store = sessions.NewCookieStore([]byte(sessionSecret))
    }()

    wg.Wait()
}

func main() {
    router := gin.Default()
    router.Use(sessions.Sessions("session", store))
    router.GET("/auth/:provider", authBeginHandler)
    router.GET("/auth/:provider/callback", completeUserAuthHandler)
    router.GET("/api/v1/data", dataHandler)
    router.GET("/api/v1/user", userHandler)
    router.GET("/debug/vars", expvar.Handler())
    router.StaticFile("/", "./public/index.html")
    log.Fatalf("Server died: %v", router.Run(listen))
}
