package jwt

import (
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"
	"gopkg.in/appleboy/gofight.v2"
	"gopkg.in/dgrijalva/jwt-go.v3"
)

var (
	key = []byte("secret key")
)

func makeTokenString(SigningAlgorithm string, username string) string {
	if SigningAlgorithm == "" {
		SigningAlgorithm = "HS256"
	}

	token := jwt.New(jwt.GetSigningMethod(SigningAlgorithm))
	claims := token.Claims.(jwt.MapClaims)
	claims["id"] = username
	claims["exp"] = time.Now().Add(time.Hour).Unix()
	claims["orig_iat"] = time.Now().Unix()
	var tokenString string
	if SigningAlgorithm == "RS256" {
		keyData, _ := ioutil.ReadFile("testdata/jwtRS256.key")
		signKey, _ := jwt.ParseRSAPrivateKeyFromPEM(keyData)
		tokenString, _ = token.SignedString(signKey)
	} else {
		tokenString, _ = token.SignedString(key)
	}

	return tokenString
}

func TestMissingRealm(t *testing.T) {

	authMiddleware := &GinJWTMiddleware{
		Key:        key,
		Timeout:    time.Hour,
		MaxRefresh: time.Hour * 24,
		Authenticator: func(userId string, password string, c *gin.Context) (interface{}, bool) {
			if userId == "admin" && password == "admin" {
				return "", true
			}

			return "", false
		},
	}

	err := authMiddleware.MiddlewareInit()

	assert.Error(t, err)
	assert.Equal(t, ErrMissingRealm, err)
}

func TestMissingKey(t *testing.T) {

	authMiddleware := &GinJWTMiddleware{
		Realm:      "test zone",
		Timeout:    time.Hour,
		MaxRefresh: time.Hour * 24,
		Authenticator: func(userId string, password string, c *gin.Context) (interface{}, bool) {
			if userId == "admin" && password == "admin" {
				return "", true
			}

			return "", false
		},
	}

	err := authMiddleware.MiddlewareInit()

	assert.Error(t, err)
	assert.Equal(t, ErrMissingSecretKey, err)
}

func TestMissingPrivKey(t *testing.T) {
	authMiddleware := &GinJWTMiddleware{
		Realm:            "zone",
		SigningAlgorithm: "RS256",
		PrivKeyFile:      "nonexisting",
	}
	err := authMiddleware.MiddlewareInit()
	assert.Error(t, err)
	assert.Equal(t, ErrNoPrivKeyFile, err)
}

func TestMissingPubKey(t *testing.T) {
	authMiddleWare := &GinJWTMiddleware{
		Realm:            "zone",
		SigningAlgorithm: "RS256",
		PrivKeyFile:      "testdata/jwtRS256.key",
		PubKeyFile:       "nonexisting",
	}
	err := authMiddleWare.MiddlewareInit()
	assert.Error(t, err)
	assert.Equal(t, ErrNoPubKeyFile, err)
}

func TestInvalidPrivKey(t *testing.T) {
	authMiddleWare := &GinJWTMiddleware{
		Realm:            "zone",
		SigningAlgorithm: "RS256",
		PrivKeyFile:      "testdata/invalidprivkey.key",
		PubKeyFile:       "testdata/jwtRS256.key.pub",
	}
	err := authMiddleWare.MiddlewareInit()
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidPrivKey, err)
}

func TestInvalidPubKey(t *testing.T) {
	authMiddleWare := &GinJWTMiddleware{
		Realm:            "zone",
		SigningAlgorithm: "RS256",
		PrivKeyFile:      "testdata/jwtRS256.key",
		PubKeyFile:       "testdata/invalidpubkey.key",
	}
	err := authMiddleWare.MiddlewareInit()
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidPubKey, err)
}

func TestMissingTimeOut(t *testing.T) {

	authMiddleware := &GinJWTMiddleware{
		Realm: "test zone",
		Key:   key,
		Authenticator: func(userId string, password string, c *gin.Context) (interface{}, bool) {
			if userId == "admin" && password == "admin" {
				return "", true
			}

			return "", false
		},
	}

	authMiddleware.MiddlewareInit()

	assert.Equal(t, time.Hour, authMiddleware.Timeout)
}

func TestMissingTokenLookup(t *testing.T) {

	authMiddleware := &GinJWTMiddleware{
		Realm: "test zone",
		Key:   key,
		Authenticator: func(userId string, password string, c *gin.Context) (interface{}, bool) {
			if userId == "admin" && password == "admin" {
				return "", true
			}

			return "", false
		},
	}

	authMiddleware.MiddlewareInit()

	assert.Equal(t, "header:Authorization", authMiddleware.TokenLookup)
}

func helloHandler(c *gin.Context) {
	c.JSON(200, gin.H{
		"text": "Hello World.",
	})
}

func ginHandler(auth *GinJWTMiddleware) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	r.POST("/login", auth.LoginHandler)

	group := r.Group("/auth")
	group.Use(auth.MiddlewareFunc())
	{
		group.GET("/hello", helloHandler)
		group.GET("/refresh_token", auth.RefreshHandler)
	}

	return r
}

func TestInternalServerError(t *testing.T) {
	// the middleware to test
	authMiddleware := &GinJWTMiddleware{}

	handler := ginHandler(authMiddleware)

	r := gofight.New()

	r.GET("/auth/hello").
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			message := gjson.Get(r.Body.String(), "message")

			assert.Equal(t, ErrMissingRealm.Error(), message.String())
			assert.Equal(t, http.StatusInternalServerError, r.Code)
		})
}

func TestErrMissingSecretKeyForLoginHandler(t *testing.T) {

	authMiddleware := &GinJWTMiddleware{
		Realm: "test zone",
		// Check key exist.
		// Key:        key,
		Timeout:    time.Hour,
		MaxRefresh: time.Hour * 24,
	}

	handler := ginHandler(authMiddleware)
	r := gofight.New()

	r.POST("/login").
		SetJSON(gofight.D{
			"username": "admin",
			"password": "admin",
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			message := gjson.Get(r.Body.String(), "message")

			assert.Equal(t, ErrMissingSecretKey.Error(), message.String())
			assert.Equal(t, http.StatusInternalServerError, r.Code)
		})
}

func TestMissingAuthenticatorForLoginHandler(t *testing.T) {

	authMiddleware := &GinJWTMiddleware{
		Realm:      "test zone",
		Key:        key,
		Timeout:    time.Hour,
		MaxRefresh: time.Hour * 24,
	}

	handler := ginHandler(authMiddleware)
	r := gofight.New()

	r.POST("/login").
		SetJSON(gofight.D{
			"username": "admin",
			"password": "admin",
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			message := gjson.Get(r.Body.String(), "message")

			assert.Equal(t, ErrMissingAuthenticatorFunc.Error(), message.String())
			assert.Equal(t, http.StatusInternalServerError, r.Code)
		})
}

func TestLoginHandler(t *testing.T) {

	// the middleware to test
	authMiddleware := &GinJWTMiddleware{
		Realm: "test zone",
		Key:   key,
		PayloadFunc: func(userId interface{}) MapClaims {
			// Set custom claim, to be checked in Authorizator method
			return MapClaims{"testkey": "testval", "exp": 0}
		},
		Authenticator: func(userId string, password string, c *gin.Context) (interface{}, bool) {
			if userId == "admin" && password == "admin" {
				return "", true
			}

			return "", false
		},
		Authorizator: func(user interface{}, c *gin.Context) bool {
			return true
		},
		LoginResponse: func(c *gin.Context, code int, token string, t time.Time) {
			c.JSON(http.StatusOK, gin.H{
				"code":    http.StatusOK,
				"token":   token,
				"expire":  t.Format(time.RFC3339),
				"message": "login successfully",
			})
		},
	}

	handler := ginHandler(authMiddleware)

	r := gofight.New()

	r.POST("/login").
		SetJSON(gofight.D{
			"username": "admin",
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			message := gjson.Get(r.Body.String(), "message")

			assert.Equal(t, ErrMissingLoginValues.Error(), message.String())
			assert.Equal(t, http.StatusBadRequest, r.Code)
			assert.Equal(t, "application/json; charset=utf-8", r.HeaderMap.Get("Content-Type"))
		})

	r.POST("/login").
		SetJSON(gofight.D{
			"username": "admin",
			"password": "test",
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			message := gjson.Get(r.Body.String(), "message")
			assert.Equal(t, ErrFailedAuthentication.Error(), message.String())
			assert.Equal(t, http.StatusUnauthorized, r.Code)
		})

	r.POST("/login").
		SetJSON(gofight.D{
			"username": "admin",
			"password": "admin",
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			message := gjson.Get(r.Body.String(), "message")
			assert.Equal(t, "login successfully", message.String())
			assert.Equal(t, http.StatusOK, r.Code)
		})
}

func performRequest(r http.Handler, method, path string, token string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest(method, path, nil)

	if token != "" {
		req.Header.Set("Authorization", token)
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	return w
}

func TestParseToken(t *testing.T) {
	// the middleware to test
	authMiddleware := &GinJWTMiddleware{
		Realm:      "test zone",
		Key:        key,
		Timeout:    time.Hour,
		MaxRefresh: time.Hour * 24,
		Authenticator: func(userId string, password string, c *gin.Context) (interface{}, bool) {
			if userId == "admin" && password == "admin" {
				return userId, true
			}

			return userId, false
		},
	}

	handler := ginHandler(authMiddleware)

	r := gofight.New()

	r.GET("/auth/hello").
		SetHeader(gofight.H{
			"Authorization": "",
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			assert.Equal(t, http.StatusUnauthorized, r.Code)
		})

	r.GET("/auth/hello").
		SetHeader(gofight.H{
			"Authorization": "Test 1234",
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			assert.Equal(t, http.StatusUnauthorized, r.Code)
		})

	r.GET("/auth/hello").
		SetHeader(gofight.H{
			"Authorization": "Bearer " + makeTokenString("HS384", "admin"),
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			assert.Equal(t, http.StatusUnauthorized, r.Code)
		})

	r.GET("/auth/hello").
		SetHeader(gofight.H{
			"Authorization": "Bearer " + makeTokenString("HS256", "admin"),
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			assert.Equal(t, http.StatusOK, r.Code)
		})
}

func TestParseTokenRS256(t *testing.T) {
	// the middleware to test
	authMiddleware := &GinJWTMiddleware{
		Realm:            "test zone",
		Key:              key,
		Timeout:          time.Hour,
		MaxRefresh:       time.Hour * 24,
		SigningAlgorithm: "RS256",
		PrivKeyFile:      "testdata/jwtRS256.key",
		PubKeyFile:       "testdata/jwtRS256.key.pub",
		Authenticator: func(userId string, password string, c *gin.Context) (interface{}, bool) {
			if userId == "admin" && password == "admin" {
				return userId, true
			}

			return userId, false
		},
	}

	handler := ginHandler(authMiddleware)

	r := gofight.New()

	r.GET("/auth/hello").
		SetHeader(gofight.H{
			"Authorization": "",
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			assert.Equal(t, http.StatusUnauthorized, r.Code)
		})

	r.GET("/auth/hello").
		SetHeader(gofight.H{
			"Authorization": "Test 1234",
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			assert.Equal(t, http.StatusUnauthorized, r.Code)
		})

	r.GET("/auth/hello").
		SetHeader(gofight.H{
			"Authorization": "Bearer " + makeTokenString("HS384", "admin"),
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			assert.Equal(t, http.StatusUnauthorized, r.Code)
		})

	r.GET("/auth/hello").
		SetHeader(gofight.H{
			"Authorization": "Bearer " + makeTokenString("RS256", "admin"),
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			assert.Equal(t, http.StatusOK, r.Code)
		})
}

func TestRefreshHandlerRS256(t *testing.T) {
	// the middleware to test
	authMiddleware := &GinJWTMiddleware{
		Realm:            "test zone",
		Key:              key,
		Timeout:          time.Hour,
		MaxRefresh:       time.Hour * 24,
		SigningAlgorithm: "RS256",
		PrivKeyFile:      "testdata/jwtRS256.key",
		PubKeyFile:       "testdata/jwtRS256.key.pub",
		Authenticator: func(userId string, password string, c *gin.Context) (interface{}, bool) {
			if userId == "admin" && password == "admin" {
				return userId, true
			}

			return userId, false
		},
		RefreshResponse: func(c *gin.Context, code int, token string, t time.Time) {
			c.JSON(http.StatusOK, gin.H{
				"code":    http.StatusOK,
				"token":   token,
				"expire":  t.Format(time.RFC3339),
				"message": "refresh successfully",
			})
		},
	}

	handler := ginHandler(authMiddleware)

	r := gofight.New()

	r.GET("/auth/refresh_token").
		SetHeader(gofight.H{
			"Authorization": "",
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			assert.Equal(t, http.StatusUnauthorized, r.Code)
		})

	r.GET("/auth/refresh_token").
		SetHeader(gofight.H{
			"Authorization": "Test 1234",
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			assert.Equal(t, http.StatusUnauthorized, r.Code)
		})
	r.GET("/auth/refresh_token").
		SetHeader(gofight.H{
			"Authorization": "Bearer " + makeTokenString("HS256", "admin"),
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			assert.Equal(t, http.StatusUnauthorized, r.Code)
		})
	r.GET("/auth/refresh_token").
		SetHeader(gofight.H{
			"Authorization": "Bearer " + makeTokenString("RS256", "admin"),
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			message := gjson.Get(r.Body.String(), "message")
			assert.Equal(t, "refresh successfully", message.String())
			assert.Equal(t, http.StatusOK, r.Code)
		})
}

func TestRefreshHandler(t *testing.T) {
	// the middleware to test
	authMiddleware := &GinJWTMiddleware{
		Realm:      "test zone",
		Key:        key,
		Timeout:    time.Hour,
		MaxRefresh: time.Hour * 24,
		Authenticator: func(userId string, password string, c *gin.Context) (interface{}, bool) {
			if userId == "admin" && password == "admin" {
				return userId, true
			}

			return userId, false
		},
	}

	handler := ginHandler(authMiddleware)

	r := gofight.New()

	r.GET("/auth/refresh_token").
		SetHeader(gofight.H{
			"Authorization": "",
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			assert.Equal(t, http.StatusUnauthorized, r.Code)
		})

	r.GET("/auth/refresh_token").
		SetHeader(gofight.H{
			"Authorization": "Test 1234",
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			assert.Equal(t, http.StatusUnauthorized, r.Code)
		})

	r.GET("/auth/refresh_token").
		SetHeader(gofight.H{
			"Authorization": "Bearer " + makeTokenString("HS256", "admin"),
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			assert.Equal(t, http.StatusOK, r.Code)
		})
}

func TestExpiredTokenOnRefreshHandler(t *testing.T) {
	// the middleware to test
	authMiddleware := &GinJWTMiddleware{
		Realm:   "test zone",
		Key:     key,
		Timeout: time.Hour,
		Authenticator: func(userId string, password string, c *gin.Context) (interface{}, bool) {
			if userId == "admin" && password == "admin" {
				return userId, true
			}

			return userId, false
		},
	}

	handler := ginHandler(authMiddleware)

	r := gofight.New()

	token := jwt.New(jwt.GetSigningMethod("HS256"))
	claims := token.Claims.(jwt.MapClaims)
	claims["id"] = "admin"
	claims["exp"] = time.Now().Add(time.Hour).Unix()
	claims["orig_iat"] = 0
	tokenString, _ := token.SignedString(key)

	r.GET("/auth/refresh_token").
		SetHeader(gofight.H{
			"Authorization": "Bearer " + tokenString,
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			assert.Equal(t, http.StatusUnauthorized, r.Code)
		})
}

func TestAuthorizator(t *testing.T) {
	// the middleware to test
	authMiddleware := &GinJWTMiddleware{
		Realm:      "test zone",
		Key:        key,
		Timeout:    time.Hour,
		MaxRefresh: time.Hour * 24,
		Authenticator: func(userId string, password string, c *gin.Context) (interface{}, bool) {
			if userId == "admin" && password == "admin" {
				return userId, true
			}
			return userId, false
		},
		Authorizator: func(user interface{}, c *gin.Context) bool {
			if user.(string) != "admin" {
				return false
			}

			return true
		},
	}

	handler := ginHandler(authMiddleware)

	r := gofight.New()

	r.GET("/auth/hello").
		SetHeader(gofight.H{
			"Authorization": "Bearer " + makeTokenString("HS256", "test"),
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			assert.Equal(t, http.StatusForbidden, r.Code)
		})

	r.GET("/auth/hello").
		SetHeader(gofight.H{
			"Authorization": "Bearer " + makeTokenString("HS256", "admin"),
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			assert.Equal(t, http.StatusOK, r.Code)
		})
}

func TestClaimsDuringAuthorization(t *testing.T) {
	// the middleware to test
	authMiddleware := &GinJWTMiddleware{
		Realm:      "test zone",
		Key:        key,
		Timeout:    time.Hour,
		MaxRefresh: time.Hour * 24,
		PayloadFunc: func(data interface{}) MapClaims {
			var testkey string
			switch data {
			case "Administrator":
				testkey = "1234"
			case "User":
				testkey = "5678"
			case "Guest":
				testkey = ""
			}
			// Set custom claim, to be checked in Authorizator method
			return MapClaims{"testkey": testkey, "exp": 0}
		},
		Authenticator: func(userId string, password string, c *gin.Context) (interface{}, bool) {
			if userId == "admin" && password == "admin" {
				return "Administrator", true
			}

			if userId == "test" && password == "test" {
				return "User", true
			}

			return "Guest", false
		},
		Authorizator: func(user interface{}, c *gin.Context) bool {
			jwtClaims := ExtractClaims(c)

			if jwtClaims["id"] == "administrator" {
				return true
			}

			if jwtClaims["testkey"] == "1234" && jwtClaims["id"] == "admin" {
				return true
			}

			if jwtClaims["testkey"] == "5678" && jwtClaims["id"] == "test" {
				return true
			}

			return false
		},
	}

	r := gofight.New()
	handler := ginHandler(authMiddleware)

	userToken, _, _ := authMiddleware.TokenGenerator("administrator", MapClaims{})

	r.GET("/auth/hello").
		SetHeader(gofight.H{
			"Authorization": "Bearer " + userToken,
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			assert.Equal(t, http.StatusOK, r.Code)
		})

	r.POST("/login").
		SetJSON(gofight.D{
			"username": "admin",
			"password": "admin",
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			token := gjson.Get(r.Body.String(), "token")
			userToken = token.String()
			assert.Equal(t, http.StatusOK, r.Code)
		})

	r.GET("/auth/hello").
		SetHeader(gofight.H{
			"Authorization": "Bearer " + userToken,
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			assert.Equal(t, http.StatusOK, r.Code)
		})

	r.POST("/login").
		SetJSON(gofight.D{
			"username": "test",
			"password": "test",
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			token := gjson.Get(r.Body.String(), "token")
			userToken = token.String()
			assert.Equal(t, http.StatusOK, r.Code)
		})

	r.GET("/auth/hello").
		SetHeader(gofight.H{
			"Authorization": "Bearer " + userToken,
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			assert.Equal(t, http.StatusOK, r.Code)
		})
}

func TestEmptyClaims(t *testing.T) {

	var jwtClaims jwt.MapClaims

	// the middleware to test
	authMiddleware := &GinJWTMiddleware{
		Realm:      "test zone",
		Key:        key,
		Timeout:    time.Hour,
		MaxRefresh: time.Hour * 24,
		Authenticator: func(userId string, password string, c *gin.Context) (interface{}, bool) {
			if userId == "admin" && password == "admin" {
				return "", true
			}

			if userId == "test" && password == "test" {
				return "Administrator", true
			}

			return userId, false
		},
		Unauthorized: func(c *gin.Context, code int, message string) {
			jwtClaims = ExtractClaims(c)
			c.String(code, message)
		},
	}

	r := gofight.New()
	handler := ginHandler(authMiddleware)

	r.GET("/auth/hello").
		SetHeader(gofight.H{
			"Authorization": "Bearer 1234",
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			assert.Equal(t, http.StatusUnauthorized, r.Code)
		})

	assert.Empty(t, jwtClaims)
}

func TestUnauthorized(t *testing.T) {
	// the middleware to test
	authMiddleware := &GinJWTMiddleware{
		Realm:      "test zone",
		Key:        key,
		Timeout:    time.Hour,
		MaxRefresh: time.Hour * 24,
		Authenticator: func(userId string, password string, c *gin.Context) (interface{}, bool) {
			if userId == "admin" && password == "admin" {
				return userId, true
			}
			return userId, false
		},
		Unauthorized: func(c *gin.Context, code int, message string) {
			c.String(code, message)
		},
	}

	handler := ginHandler(authMiddleware)

	r := gofight.New()

	r.GET("/auth/hello").
		SetHeader(gofight.H{
			"Authorization": "Bearer 1234",
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			assert.Equal(t, http.StatusUnauthorized, r.Code)
		})
}

func TestTokenExpire(t *testing.T) {
	// the middleware to test
	authMiddleware := &GinJWTMiddleware{
		Realm:      "test zone",
		Key:        key,
		Timeout:    time.Hour,
		MaxRefresh: -time.Second,
		Authenticator: func(userId string, password string, c *gin.Context) (interface{}, bool) {
			if userId == "admin" && password == "admin" {
				return userId, true
			}
			return userId, false
		},
		Unauthorized: func(c *gin.Context, code int, message string) {
			c.String(code, message)
		},
	}

	handler := ginHandler(authMiddleware)

	r := gofight.New()

	userToken, _, _ := authMiddleware.TokenGenerator("admin", MapClaims{})

	r.GET("/auth/refresh_token").
		SetHeader(gofight.H{
			"Authorization": "Bearer " + userToken,
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			assert.Equal(t, http.StatusUnauthorized, r.Code)
		})
}

func TestTokenFromQueryString(t *testing.T) {
	// the middleware to test
	authMiddleware := &GinJWTMiddleware{
		Realm:   "test zone",
		Key:     key,
		Timeout: time.Hour,
		Authenticator: func(userId string, password string, c *gin.Context) (interface{}, bool) {
			if userId == "admin" && password == "admin" {
				return userId, true
			}
			return userId, false
		},
		Unauthorized: func(c *gin.Context, code int, message string) {
			c.String(code, message)
		},
		TokenLookup: "query:token",
	}

	handler := ginHandler(authMiddleware)

	r := gofight.New()

	userToken, _, _ := authMiddleware.TokenGenerator("admin", MapClaims{})

	r.GET("/auth/refresh_token").
		SetHeader(gofight.H{
			"Authorization": "Bearer " + userToken,
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			assert.Equal(t, http.StatusUnauthorized, r.Code)
		})

	r.GET("/auth/refresh_token?token="+userToken).
		SetHeader(gofight.H{
			"Authorization": "Bearer " + userToken,
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			assert.Equal(t, http.StatusOK, r.Code)
		})
}

func TestTokenFromCookieString(t *testing.T) {
	// the middleware to test
	authMiddleware := &GinJWTMiddleware{
		Realm:   "test zone",
		Key:     key,
		Timeout: time.Hour,
		Authenticator: func(userId string, password string, c *gin.Context) (interface{}, bool) {
			if userId == "admin" && password == "admin" {
				return userId, true
			}
			return userId, false
		},
		Unauthorized: func(c *gin.Context, code int, message string) {
			c.String(code, message)
		},
		TokenLookup: "cookie:token",
	}

	handler := ginHandler(authMiddleware)

	r := gofight.New()

	userToken, _, _ := authMiddleware.TokenGenerator("admin", MapClaims{})

	r.GET("/auth/refresh_token").
		SetHeader(gofight.H{
			"Authorization": "Bearer " + userToken,
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			assert.Equal(t, http.StatusUnauthorized, r.Code)
		})

	r.GET("/auth/refresh_token").
		SetCookie(gofight.H{
			"token": userToken,
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			assert.Equal(t, http.StatusOK, r.Code)
		})
}

func TestDefineTokenHeadName(t *testing.T) {
	// the middleware to test
	authMiddleware := &GinJWTMiddleware{
		Realm:         "test zone",
		Key:           key,
		Timeout:       time.Hour,
		TokenHeadName: "JWTTOKEN       ",
		Authenticator: func(userId string, password string, c *gin.Context) (interface{}, bool) {
			if userId == "admin" && password == "admin" {
				return userId, true
			}
			return userId, false
		},
	}

	handler := ginHandler(authMiddleware)

	r := gofight.New()

	r.GET("/auth/hello").
		SetHeader(gofight.H{
			"Authorization": "Bearer " + makeTokenString("HS256", "admin"),
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			assert.Equal(t, http.StatusUnauthorized, r.Code)
		})

	r.GET("/auth/hello").
		SetHeader(gofight.H{
			"Authorization": "JWTTOKEN " + makeTokenString("HS256", "admin"),
		}).
		Run(handler, func(r gofight.HTTPResponse, rq gofight.HTTPRequest) {
			assert.Equal(t, http.StatusOK, r.Code)
		})
}

func TestHTTPStatusMessageFunc(t *testing.T) {
	var successError = errors.New("Successful test error")
	var failedError = errors.New("Failed test error")
	var successMessage = "Overwrite error message."

	authMiddleware := &GinJWTMiddleware{
		Key:        key,
		Timeout:    time.Hour,
		MaxRefresh: time.Hour * 24,
		Authenticator: func(userId string, password string, c *gin.Context) (interface{}, bool) {
			if userId == "admin" && password == "admin" {
				return "", true
			}

			return "", false
		},

		HTTPStatusMessageFunc: func(e error, c *gin.Context) string {
			if e == successError {
				return successMessage
			}

			return e.Error()
		},
	}

	successString := authMiddleware.HTTPStatusMessageFunc(successError, nil)
	failedString := authMiddleware.HTTPStatusMessageFunc(failedError, nil)

	assert.Equal(t, successMessage, successString)
	assert.NotEqual(t, successMessage, failedString)
}
