package main

import (
	"context"
	"errors"
	"html/template"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var DB *gorm.DB

type IDs struct {
	TopicID   string `param:"topicid"`
	PostID    string `param:"postid"`
	CommentID string `param:"commentid"`
}
type Model struct {
	ID        string `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}
type Topic struct {
	Model
	Posts []Post `json:"posts"`
}
type Post struct {
	Model
	TopicID  string    `gorm:"primaryKey" json:"topicID"`
	Title    string    `json:"title"`
	Content  string    `json:"content"`
	Votes    int       `json:"votes"`
	Comments []Comment `json:"comments"`
}
type Comment struct {
	Model
	TopicID string `gorm:"primaryKey" json:"topicID"`
	PostID  string `gorm:"primaryKey" json:"postID"`
	Content string `json:"content"`
	Votes   int    `json:"votes"`
}
type CreateRequest[T any] struct {
	IDs
	Model T `json:"model"`
}
type UpdateRequest[T any] struct {
	IDs
	Mask T `json:"updateMask"`
}
type GetRequest struct {
	IDs
}
type ListRequest struct {
	IDs
}
type DeleteRequest struct {
	IDs
}
type Template struct {
	templates *template.Template
}
type CreateCommentRequest struct {
	IDs
	Content string `form:"content"`
}
type CreatePostRequest struct {
	IDs
	Title   string `form:"title"`
	Content string `form:"content"`
}
type CreateTopicRequest struct {
	ID string `form:"id"`
}

func (t *Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}
func V1[T any, R any](f func(context.Context, R) (T, error)) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req R
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, err)
		}
		if obj, err := f(c.Request().Context(), req); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
			}
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		} else {
			return c.JSON(http.StatusOK, obj)
		}
	}
}
func Serve[T any](template string, f func(IDs) T, preloads ...string) echo.HandlerFunc {
	return func(c echo.Context) error {
		var ids IDs
		if err := c.Bind(&ids); err != nil {
			return c.JSON(http.StatusBadRequest, err)
		}
		obj, err := Get(c.Request().Context(), f(ids), preloads...)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
			}
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return c.Render(http.StatusOK, template, obj)
	}
}
func Get[T any](c context.Context, id T, preloads ...string) (*T, error) {
	var obj T
	query := DB.Where(&id)
	for _, preload := range preloads {
		query.Preload(preload)
	}
	return &obj, query.First(&obj).Error
}
func Create[T any](c context.Context, obj T) (*T, error) {
	return &obj, DB.Create(&obj).Error
}
func Update[T any](c context.Context, model T, mask T) (*T, error) {
	if res := DB.Model(&model).Updates(mask); res.Error != nil {
		return new(T), res.Error
	}
	if obj, err := Get(c, model); err != nil {
		return new(T), err
	} else {
		return obj, nil
	}
}
func List[T any](c context.Context, id T, objs []T) (*[]T, error) {
	return &objs, DB.Where(id).Find(&objs).Error
}
func Delete[T any](c context.Context, id T) (*T, error) {
	return new(T), DB.Where(id).Delete(new(T), id).Error
}
func HandleCreate[T any, R any](f func(R) T) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req R
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		obj, err := Create(c.Request().Context(), f(req))
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, obj)
	}
}
func HandleVote[T any](f func(IDs) T, upVote func(*T) int) echo.HandlerFunc {
	return func(c echo.Context) error {
		var id IDs
		if err := c.Bind(&id); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		obj, err := Get(c.Request().Context(), f(id))
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		err = DB.Model(&obj).Update("votes", upVote(obj)).Error
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, map[string]string{})
	}
}
func main() {
	db, err := gorm.Open(sqlite.Open("tmp/test.db"), &gorm.Config{TranslateError: true})
	if err != nil {
		log.Fatalf("failed to open gorm: %s", err.Error())
	}
	db.AutoMigrate(&Post{}, &Comment{}, &Topic{})
	DB = db
	t := &Template{templates: template.Must(template.ParseGlob("web/views/*.html"))}
	e := echo.New()
	e.Renderer = t
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.GET("/", func(c echo.Context) error {
		topics, err := List(c.Request().Context(), Topic{}, []Topic{})
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return c.Render(http.StatusOK, "index", topics)
	})
	e.GET("/topics/:topicid", Serve("topic", func(i IDs) Topic { return Topic{Model: Model{ID: i.TopicID}} }, "Posts"))
	e.GET("/topics/:topicid/posts/:postid", Serve("post", func(i IDs) Post { return Post{Model: Model{ID: i.PostID}, TopicID: i.TopicID} }, "Comments"))
	e.POST("/topics", HandleCreate(func(req CreateTopicRequest) Topic { return Topic{Model: Model{ID: req.ID}} }))
	e.POST("/topics/:topicid/posts", HandleCreate(func(req CreatePostRequest) Post {
		return Post{Model: Model{ID: uuid.NewString()}, TopicID: req.TopicID, Title: req.Title, Content: req.Content}
	}))
	e.POST("/topics/:topicid/posts/:postid/comments", HandleCreate(func(req CreateCommentRequest) Comment {
		return Comment{Model: Model{ID: uuid.NewString()}, TopicID: req.TopicID, PostID: req.PostID, Content: req.Content}
	}))
	e.POST("/topics/:topicid/posts/:postid/comments/:commentid/upvote", HandleVote(func(id IDs) Comment {
		return Comment{Model: Model{ID: id.CommentID}, TopicID: id.TopicID, PostID: id.PostID}
	}, func(comment *Comment) int { return comment.Votes + 1 }))
	e.POST("/topics/:topicid/posts/:postid/comments/:commentid/downvote", HandleVote(func(id IDs) Comment {
		return Comment{Model: Model{ID: id.CommentID}, TopicID: id.TopicID, PostID: id.PostID}
	}, func(comment *Comment) int { return comment.Votes - 1 }))
	e.POST("/topics/:topicid/posts/:postid/upvote", HandleVote(func(id IDs) Post { return Post{Model: Model{ID: id.PostID}, TopicID: id.TopicID} }, func(post *Post) int { return post.Votes + 1 }))
	e.POST("/topics/:topicid/posts/:postid/downvote", HandleVote(func(id IDs) Post { return Post{Model: Model{ID: id.PostID}, TopicID: id.TopicID} }, func(post *Post) int { return post.Votes - 1 }))

	// e.POST("/v1/topics", V1(func(c context.Context, req CreateRequest[Topic]) (*Topic, error) {
	// 	return Create(c, Topic{Model: Model{ID: req.Model.ID}})
	// }))
	// e.GET("/v1/topics/:topicid", V1(func(c context.Context, req GetRequest) (*Topic, error) {
	// 	return Get(c, Topic{Model: Model{ID: req.TopicID}}, "Posts")
	// }))
	// e.GET("/v1/topics", V1(func(c context.Context, req ListRequest) (*[]Topic, error) { return List(c, Topic{}, []Topic{}) }))
	// e.DELETE("/v1/topics/:topicid", V1(func(c context.Context, req DeleteRequest) (*Topic, error) {
	// 	return Delete(c, Topic{Model: Model{ID: req.TopicID}})
	// }))
	// e.POST("/v1/topics/:topicid/posts", V1(func(c context.Context, req CreateRequest[Post]) (*Post, error) {
	// 	return Create(c, Post{Model: Model{ID: uuid.NewString()}, TopicID: req.TopicID, Title: req.Model.Title, Content: req.Model.Content})
	// }))
	// e.PUT("/v1/topics/:topicid/posts/:postid", V1(func(c context.Context, req UpdateRequest[Post]) (*Post, error) {
	// 	return Update(c, Post{Model: Model{ID: req.PostID}, TopicID: req.TopicID}, req.Mask)
	// }))
	// e.GET("/v1/topics/:topicid/posts/:postid", V1(func(c context.Context, req GetRequest) (*Post, error) {
	// 	return Get(c, Post{Model: Model{ID: req.PostID}, TopicID: req.TopicID})
	// }))
	// e.GET("/v1/topics/:topicid/posts", V1(func(c context.Context, req ListRequest) (*[]Post, error) {
	// 	return List(c, Post{TopicID: req.TopicID}, []Post{})
	// }))
	// e.DELETE("/v1/topics/:topicid/posts/:postid", V1(func(c context.Context, req DeleteRequest) (*Post, error) {
	// 	return Delete(c, Post{Model: Model{ID: req.PostID}, TopicID: req.TopicID})
	// }))
	// e.POST("/v1/topics/:topicid/posts/:postid/comments", V1(func(c context.Context, req CreateRequest[Comment]) (*Comment, error) {
	// 	return Create(c, Comment{Model: Model{ID: uuid.NewString()}, TopicID: req.TopicID, PostID: req.PostID, Content: req.Model.Content})
	// }))
	// e.PUT("/v1/topics/:topicid/posts/:postid/comments/:commentid", V1(func(c context.Context, req UpdateRequest[Comment]) (*Comment, error) {
	// 	return Update(c, Comment{Model: Model{ID: req.CommentID}, TopicID: req.TopicID, PostID: req.PostID}, req.Mask)
	// }))
	// e.GET("/v1/topics/:topicid/posts/:postid/comments/:commentid", V1(func(c context.Context, req GetRequest) (*Comment, error) {
	// 	return Get(c, Comment{Model: Model{ID: req.CommentID}, TopicID: req.TopicID, PostID: req.PostID})
	// }))
	// e.GET("/v1/topics/:topicid/posts/:postid/comments", V1(func(c context.Context, req ListRequest) (*[]Comment, error) {
	// 	return List(c, Comment{TopicID: req.TopicID, PostID: req.PostID}, []Comment{})
	// }))
	// e.DELETE("/v1/topics/:topicid/posts/:postid/comments/:commentid", V1(func(c context.Context, req DeleteRequest) (*Comment, error) {
	// 	return Delete(c, Comment{Model: Model{ID: req.CommentID}, TopicID: req.TopicID, PostID: req.PostID})
	// }))
	e.Logger.Fatal(e.Start("127.0.0.1:9001"))
}
