package main

import (
    "encoding/json"
    "errors"
    "fmt"
    "log"
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/gin-gonic/contrib/sessions"

    "github.com/markbates/goth"
    "github.com/markbates/goth/gothic"
)

var (
    // GothicSessionName is the session key name for storing the Gothic session.
    GothicSessionName = gothic.SessionName
)

func writeJSON(c *gin.Context, data interface{}) error {
    c.Writer.Header().Set("Content-Type", "application/json")
    if data != nil {
        enc := json.NewEncoder(c.Writer)
        if err := enc.Encode(data); err != nil {
            return err
        }
    }
    return nil
}

// getUser gets the user from the current request. Could be a middleware unto itself, but because there's only one real hander that would be overkill.
func getUser(c *gin.Context) (*User, error) {

    session := sessions.Default(c)
    v := session.Get("user.id")
    if v == nil {
        return nil, errors.New(http.StatusText(http.StatusUnauthorized))
    }

    u := &User{}
    if err := db.First(u, v).Error; err != nil {
        return nil, err
    }

    return u, nil
}

func dataHandler(c *gin.Context) {

    var data []Data
    u, err := getUser(c)
    if err != nil {
        log.Printf("[DEBUG] %v", err)
        http.Error(c.Writer, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
        return
    }

    log.Printf("[DEBUG] %+v %v", u, u.Type)
    if err := db.Where("type <= ?", u.Type).Find(&data).Error; err != nil {
        http.Error(c.Writer, err.Error(), http.StatusInternalServerError)
        return
    }

    writeJSON(c, data)
}

func userHandler(c *gin.Context) {

    u, err := getUser(c)
    if err != nil {
        log.Printf("[DEBUG] %v", err)
        http.Error(c.Writer, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
        return
    }

    writeJSON(c, u)
    return
}

func getState(req *http.Request) string {
    state := req.URL.Query().Get("state")
	if len(state) > 0 {
		return state
	}
	return "state"
}

func authBeginHandler(c *gin.Context) {

    providerName := c.Params.ByName("provider")
    provider, err := goth.GetProvider(providerName)
    if err != nil {
        return
    }

    sess, err := provider.BeginAuth(getState(c.Request))
    if err != nil {
        return
    }

    url, err := sess.GetAuthURL()
	if err != nil {
		return
	}

    session := sessions.Default(c)
    session.Set(GothicSessionName, sess.Marshal())
    session.Save()

	http.Redirect(c.Writer, c.Request, url, http.StatusTemporaryRedirect)
}

func completeUserAuthHandler(c *gin.Context) {

    defer func() {
        if err := recover(); err != nil {
            log.Printf("completeUserAuthHandler error: %v", err)
            http.Redirect(c.Writer, c.Request, fmt.Sprintf("/#/?error=%v", err), http.StatusTemporaryRedirect)
            return
       }
    }()

    session := sessions.Default(c)
    providerName := c.Params.ByName("provider")
    provider, err := goth.GetProvider(providerName)
	if err != nil {
		panic(err)
	}

    v := session.Get(GothicSessionName)
    if v == nil {
        panic(errors.New("Unable to get session"))
    }

    sess, err := provider.UnmarshalSession(v.(string))
    if err != nil {
        panic(err)
    }

    _, err = sess.Authorize(provider, c.Request.URL.Query())
    if err != nil {
        panic(err)
    }

    // Fetch the user.
	auth, err := provider.FetchUser(sess)
    if err != nil {
        panic(err)
    }

    // Check if they exist.
    u := &User{}
    if err = db.First(u, "email = ?", auth.Email).Error; err != nil {
        panic(fmt.Sprintf("User %s isn't registered", auth.Email))
    }

    session.Set("user.id", u.ID)
    session.Save()
    http.Redirect(c.Writer, c.Request, "/", http.StatusTemporaryRedirect)
}
