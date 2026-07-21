package blog

import (
	"testing"
	"time"

	"mu/internal/data"
)

func TestDeletePostsByAuthorHydratesPersistedPostsBeforeLoad(t *testing.T) {
	deleted := &Post{ID: "deleted-post", AuthorID: "deleted", CreatedAt: time.Now()}
	survivor := &Post{ID: "survivor-post", AuthorID: "survivor", CreatedAt: time.Now()}
	if err := data.SaveJSON("blog.json", []*Post{deleted, survivor}); err != nil {
		t.Fatal(err)
	}
	if err := data.SaveJSON("comments.json", []*Comment{
		{ID: "deleted-comment", PostID: deleted.ID, AuthorID: "deleted"},
		{ID: "survivor-comment", PostID: survivor.ID, AuthorID: "survivor"},
	}); err != nil {
		t.Fatal(err)
	}

	mutex.Lock()
	oldPosts, oldPostsMap, oldComments := posts, postsMap, comments
	posts, postsMap, comments = nil, nil, nil
	mutex.Unlock()
	t.Cleanup(func() {
		mutex.Lock()
		posts, postsMap, comments = oldPosts, oldPostsMap, oldComments
		mutex.Unlock()
	})

	if err := DeletePostsByAuthor("deleted"); err != nil {
		t.Fatal(err)
	}

	var gotPosts []*Post
	if err := data.LoadJSON("blog.json", &gotPosts); err != nil {
		t.Fatal(err)
	}
	if len(gotPosts) != 1 || gotPosts[0].AuthorID != "survivor" {
		t.Fatalf("posts after pre-load cleanup = %#v, want only survivor", gotPosts)
	}
	var gotComments []*Comment
	if err := data.LoadJSON("comments.json", &gotComments); err != nil {
		t.Fatal(err)
	}
	if len(gotComments) != 1 || gotComments[0].AuthorID != "survivor" {
		t.Fatalf("comments after pre-load cleanup = %#v, want only survivor", gotComments)
	}
}
