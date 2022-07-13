// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package edgeschema

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"entgo.io/ent/entc/integration/edgeschema/ent"
	"entgo.io/ent/entc/integration/edgeschema/ent/friendship"
	"entgo.io/ent/entc/integration/edgeschema/ent/group"
	"entgo.io/ent/entc/integration/edgeschema/ent/migrate"
	"entgo.io/ent/entc/integration/edgeschema/ent/relationship"
	_ "entgo.io/ent/entc/integration/edgeschema/ent/runtime"
	"entgo.io/ent/entc/integration/edgeschema/ent/tweetlike"
	"entgo.io/ent/entc/integration/edgeschema/ent/user"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func TestEdgeSchemaWithID(t *testing.T) {
	client, err := ent.Open(dialect.SQLite, "file:ent?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	defer client.Close()
	ctx := context.Background()
	require.NoError(t, client.Schema.Create(ctx, migrate.WithGlobalUniqueID(true)))

	hub, lab := client.Group.Create().SetName("GitHub").SaveX(ctx), client.Group.Create().SetName("GitLab").SaveX(ctx)
	a8m, nat := client.User.Create().SetName("a8m").AddGroups(hub, lab).SaveX(ctx), client.User.Create().SetName("nati").AddGroups(hub).SaveX(ctx)
	require.Equal(t, 2, a8m.QueryGroups().CountX(ctx))
	require.Equal(t, 1, nat.QueryGroups().CountX(ctx))

	edges := a8m.QueryJoinedGroups().AllX(ctx)
	require.Equal(t, a8m.ID, edges[0].UserID)
	require.Equal(t, hub.ID, edges[0].GroupID)
	require.False(t, edges[0].JoinedAt.IsZero())
	require.Equal(t, a8m.ID, edges[1].UserID)
	require.Equal(t, lab.ID, edges[1].GroupID)
	require.False(t, edges[1].JoinedAt.IsZero())
	require.Equal(t, hub.ID, a8m.QueryJoinedGroups().QueryGroup().FirstIDX(ctx))
	require.Equal(t, lab.ID, a8m.QueryJoinedGroups().QueryGroup().Order(ent.Desc(group.FieldID)).FirstIDX(ctx))

	edges = nat.QueryJoinedGroups().AllX(ctx)
	require.Equal(t, nat.ID, edges[0].UserID)
	require.Equal(t, hub.ID, edges[0].GroupID)
	require.False(t, edges[0].JoinedAt.IsZero())

	err = nat.Update().AddGroups(hub).Exec(ctx)
	require.True(t, ent.IsConstraintError(err), "unique constraint failed: user_groups.user_id, user_groups.group_id")

	users := client.User.Query().WithJoinedGroups(func(q *ent.UserGroupQuery) { q.WithGroup() }).AllX(ctx)
	require.Equal(t, []int{a8m.ID, nat.ID}, []int{users[0].ID, users[1].ID})
	require.Equal(t, []int{hub.ID, lab.ID}, []int{users[0].Edges.JoinedGroups[0].GroupID, users[0].Edges.JoinedGroups[1].GroupID})
	require.Equal(t, []int{hub.ID, lab.ID}, []int{users[0].Edges.JoinedGroups[0].Edges.Group.ID, users[0].Edges.JoinedGroups[1].Edges.Group.ID})
	require.Equal(t, hub.ID, users[1].Edges.JoinedGroups[0].GroupID)

	// Ignore update as we already have such edge between a8m and hub.
	client.UserGroup.Create().SetUser(a8m).SetGroup(hub).OnConflict().Ignore().ExecX(ctx)
}

func TestEdgeSchemaCompositeID(t *testing.T) {
	client, err := ent.Open(dialect.SQLite, "file:ent?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	defer client.Close()
	ctx := context.Background()
	require.NoError(t, client.Schema.Create(ctx, migrate.WithGlobalUniqueID(true)))

	tweets := client.Tweet.CreateBulk(
		client.Tweet.Create().SetText("foo"),
		client.Tweet.Create().SetText("bar"),
		client.Tweet.Create().SetText("baz"),
	).SaveX(ctx)
	a8m := client.User.Create().SetName("a8m").AddLikedTweets(tweets[:2]...).SaveX(ctx)
	nat := client.User.Create().SetName("nati").AddLikedTweets(tweets[0]).SaveX(ctx)
	likes := a8m.QueryLikes().AllX(ctx)
	require.Len(t, likes, 2)
	require.Equal(t, a8m.ID, likes[0].UserID)
	require.Equal(t, tweets[0].ID, likes[0].TweetID)
	require.Equal(t, a8m.ID, likes[1].UserID)
	require.Equal(t, tweets[1].ID, likes[1].TweetID)
	ts := time.Unix(1653377090, 0)
	like := client.TweetLike.Create().SetUser(a8m).SetLikedAt(ts).SetTweet(tweets[2]).SaveX(ctx)
	require.Equal(t, a8m.ID, like.UserID)
	require.Equal(t, tweets[2].ID, like.TweetID)
	require.Equal(t, a8m.ID, like.QueryUser().OnlyIDX(ctx))
	require.Equal(t, tweets[2].ID, like.QueryTweet().OnlyIDX(ctx))
	require.Equal(t, 3, a8m.QueryLikes().CountX(ctx))
	require.Equal(t, []int{tweets[0].ID, tweets[1].ID, tweets[2].ID}, a8m.QueryLikes().QueryTweet().IDsX(ctx))
	for _, k := range []*ent.TweetLike{
		a8m.QueryLikes().Where(tweetlike.LikedAt(ts)).OnlyX(ctx),
		client.TweetLike.Query().Where(tweetlike.LikedAt(ts)).OnlyX(ctx),
		client.Tweet.Query().QueryLikes().Where(tweetlike.LikedAt(ts)).OnlyX(ctx),
		client.Tweet.Query().QueryLikes().Where(tweetlike.LikedAt(ts), tweetlike.HasUserWith(user.Name(a8m.Name))).OnlyX(ctx),
		client.User.Query().QueryLikedTweets().QueryLikes().Where(tweetlike.LikedAt(ts), tweetlike.HasUserWith(user.Name(a8m.Name))).OnlyX(ctx),
	} {
		require.Equal(t, like.UserID, k.UserID)
		require.Equal(t, like.TweetID, k.TweetID)
		require.Equal(t, like.LikedAt.Unix(), k.LikedAt.Unix())
	}
	nat = nat.Update().AddLikedTweetIDs(like.TweetID).SaveX(ctx)
	require.Equal(t, 2, nat.QueryLikes().CountX(ctx))
	require.Equal(t, 5, client.TweetLike.Query().CountX(ctx))
	require.Equal(t, 3, client.TweetLike.Query().Where(tweetlike.HasUserWith(user.Name(a8m.Name))).CountX(ctx))
	require.Equal(t, 2, client.TweetLike.Query().Where(tweetlike.HasUserWith(user.Name(nat.Name))).CountX(ctx))

	var v []struct {
		UserID int `sql:"user_id"`
		Count  int `sql:"count"`
	}
	client.TweetLike.Query().GroupBy(tweetlike.FieldUserID).Aggregate(ent.Count()).ScanX(ctx, &v)
	require.Equal(t, a8m.ID, v[0].UserID)
	require.Equal(t, 3, v[0].Count)
	require.Equal(t, nat.ID, v[1].UserID)
	require.Equal(t, 2, v[1].Count)

	// Ignore update as we already have such edge between a8m and hub.
	client.TweetLike.Create().SetUserID(like.UserID).SetTweetID(like.TweetID).OnConflict().Ignore().ExecX(ctx)
	client.TweetLike.Create().SetUserID(like.UserID).SetTweetID(like.TweetID).OnConflict().DoNothing().ExecX(ctx)

	// Clean all tweet likes and create them in batch again.
	client.TweetLike.Delete().ExecX(ctx)
	likes = client.TweetLike.CreateBulk(
		client.TweetLike.Create().SetUserID(a8m.ID).SetTweet(tweets[0]),
		client.TweetLike.Create().SetUserID(a8m.ID).SetTweet(tweets[1]),
		client.TweetLike.Create().SetUserID(nat.ID).SetTweet(tweets[1]),
		client.TweetLike.Create().SetUserID(nat.ID).SetTweet(tweets[2]),
	).SaveX(ctx)
	require.Equal(t, likes[0].UserID, a8m.ID)
	require.Equal(t, likes[0].TweetID, tweets[0].ID)
	require.NotZero(t, likes[0].LikedAt)
	require.Equal(t, likes[1].UserID, a8m.ID)
	require.Equal(t, likes[1].TweetID, tweets[1].ID)
	require.NotZero(t, likes[1].LikedAt)

	require.Equal(t, likes[2].UserID, nat.ID)
	require.Equal(t, likes[2].TweetID, tweets[1].ID)
	require.NotZero(t, likes[2].LikedAt)
	require.Equal(t, likes[3].UserID, nat.ID)
	require.Equal(t, likes[3].TweetID, tweets[2].ID)
	require.NotZero(t, likes[3].LikedAt)
}

func TestEdgeSchemaDefaultID(t *testing.T) {
	client, err := ent.Open(dialect.SQLite, "file:ent?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	defer client.Close()
	ctx := context.Background()
	require.NoError(t, client.Schema.Create(ctx))

	tweet1 := client.Tweet.Create().SetText("foo").SaveX(ctx)
	tag1 := client.Tag.Create().SetValue("1").SaveX(ctx)
	tweet1.Update().AddTags(tag1).SaveX(ctx)
	require.Equal(t, tag1.ID, tweet1.QueryTags().OnlyIDX(ctx))
	require.NotEqual(t, uuid.Nil, tweet1.QueryTweetTags().OnlyIDX(ctx))

	tweet2 := client.Tweet.Create().SetText("bar").AddTags(tag1).SaveX(ctx)
	require.Equal(t, tag1.ID, tweet2.QueryTags().OnlyIDX(ctx))
	require.NotEqual(t, uuid.Nil, tweet2.QueryTweetTags().OnlyIDX(ctx))
}

func TestEdgeSchemaBidiWithID(t *testing.T) {
	client, err := ent.Open(dialect.SQLite, "file:ent?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	defer client.Close()
	ctx := context.Background()
	require.NoError(t, client.Schema.Create(ctx, migrate.WithGlobalUniqueID(true)))

	nat := client.User.Create().SetName("nati").SaveX(ctx)
	a8m := client.User.Create().SetName("a8m").AddFriends(nat).SaveX(ctx)
	for _, f1 := range []*ent.Friendship{
		a8m.QueryFriendships().OnlyX(ctx),
		nat.QueryFriendships().QueryFriend().QueryFriendships().OnlyX(ctx),
		client.Friendship.Query().Where(friendship.HasFriendWith(user.Name(nat.Name))).OnlyX(ctx),
	} {
		require.Equal(t, friendship.DefaultWeight, f1.Weight)
		require.False(t, f1.CreatedAt.IsZero())
		require.Equal(t, a8m.ID, f1.UserID)
		require.Equal(t, nat.ID, f1.FriendID)
	}
	require.Equal(t, 2, client.Friendship.Query().CountX(ctx), "bidirectional edges create 2 records in the join table")
}

func TestEdgeSchemaBidiCompositeID(t *testing.T) {
	client, err := ent.Open(dialect.SQLite, "file:ent?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	defer client.Close()
	ctx := context.Background()
	require.NoError(t, client.Schema.Create(ctx, migrate.WithGlobalUniqueID(true)))

	u1 := client.User.Create().SetName("u1").SaveX(ctx)
	u2 := client.User.Create().SetName("u2").AddRelatives(u1).SaveX(ctx)
	u3 := client.User.Create().SetName("u3").AddRelatives(u2).SaveX(ctx)

	var v []struct {
		UserID int `sql:"user_id"`
		Count  int `sql:"count"`
	}
	client.Relationship.Query().GroupBy(relationship.FieldUserID).Aggregate(ent.Count()).ScanX(ctx, &v)
	require.EqualValues(
		t,
		[]struct{ UserID, Count int }{{u1.ID, 1}, {u2.ID, 2}, {u3.ID, 1}},
		v,
	)
	for _, r := range []int{
		u2.QueryRelationship().Where(relationship.RelativeID(u3.ID)).QueryRelative().OnlyIDX(ctx),
		u1.QueryRelatives().QueryRelationship().Where(relationship.RelativeIDNEQ(u1.ID)).QueryRelative().OnlyIDX(ctx),
		client.User.Query().Where(user.ID(u1.ID)).QueryRelatives().QueryRelationship().Where(relationship.RelativeIDNEQ(u1.ID)).QueryRelative().OnlyIDX(ctx),
	} {
		require.Equal(t, u3.ID, r)
	}

	info := client.RelationshipInfo.Create().SetText("u1->u2").SaveX(ctx)
	r1 := u1.QueryRelationship().OnlyX(ctx)
	r1.Update().SetInfo(info).ExecX(ctx)
	r2 := client.User.Query().QueryRelationship().Where(relationship.HasInfo()).WithInfo().OnlyX(ctx)
	require.Equal(t, r1.UserID, r2.UserID)
	require.Equal(t, r1.RelativeID, r2.RelativeID)
	require.Equal(t, info.ID, r2.Edges.Info.ID)
}

func TestEdgeSchemaForO2M(t *testing.T) {
	client, err := ent.Open(dialect.SQLite, "file:ent?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	defer client.Close()
	ctx := context.Background()
	require.NoError(t, client.Schema.Create(ctx, migrate.WithGlobalUniqueID(true)))

	t1 := client.Tweet.Create().SetText("Hello Edge Schema").SaveX(ctx)
	a8m := client.User.Create().SetName("a8m").AddTweets(t1).SaveX(ctx)
	require.Equal(t, t1.ID, a8m.QueryTweets().OnlyIDX(ctx))
	_, err = client.User.Create().SetName("nati").AddTweets(t1).Save(ctx)
	require.True(t, ent.IsConstraintError(err), "Tweet can have only one author")

	nat := client.User.Create().SetName("nati").SaveX(ctx)
	err = nat.Update().AddTweets(t1).Exec(ctx)
	require.True(t, ent.IsConstraintError(err))
	err = client.UserTweet.Create().SetUser(nat).SetTweet(t1).Exec(ctx)
	require.True(t, ent.IsConstraintError(err))

	tweets := client.Tweet.CreateBulk(
		client.Tweet.Create().SetText("t1"),
		client.Tweet.Create().SetText("t2"),
	).SaveX(ctx)
	nat.Update().AddTweets(tweets...).ExecX(ctx)
}