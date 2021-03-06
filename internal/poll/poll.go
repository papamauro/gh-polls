package poll

import (
	"math/rand"
	"strings"
	"time"

	"github.com/oklog/ulid"
	"github.com/pkg/errors"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
)

// Config.
var (
	table   = "polls"
	client  = dynamodb.New(session.New(aws.NewConfig()))
	entropy = rand.New(rand.NewSource(time.Now().UnixNano()))
)

// Errors.
var (
	ErrAlreadyVoted = errors.New("already voted")
)

// newID returns a new id.
func newID() string {
	return ulid.MustNew(ulid.Now(), entropy).String()
}

// Poll represents a poll and its options.
type Poll struct {
	ID      string         `json:"id"`
	User    string         `json:"user"`
	Votes   int            `json:"votes"`
	Voters  []string       `json:"voters"`
	Options map[string]int `json:"options"`
	options []string
}

// New poll for the given user and options.
func New(user string, options []string) *Poll {
	return &Poll{
		ID:      newID(),
		User:    user,
		options: options,
	}
}

// Create the poll.
func (p *Poll) Create() error {
	opts := map[string]*dynamodb.AttributeValue{}

	for _, name := range p.options {
		opts[name] = &dynamodb.AttributeValue{
			N: aws.String("0"),
		}
	}

	item := map[string]*dynamodb.AttributeValue{
		"id": {
			S: &p.ID,
		},
		"user": {
			S: &p.User,
		},
		"options": {
			M: opts,
		},
	}

	_, err := client.PutItem(&dynamodb.PutItemInput{
		TableName: &table,
		Item:      item,
	})

	return err
}

// Remove the poll.
func (p *Poll) Remove() error {
	key := map[string]*dynamodb.AttributeValue{
		"id": {
			S: &p.ID,
		},
	}

	_, err := client.DeleteItem(&dynamodb.DeleteItemInput{
		TableName: &table,
		Key:       key,
	})

	return err
}

// Load the poll.
func (p *Poll) Load() error {
	key := map[string]*dynamodb.AttributeValue{
		"id": {
			S: &p.ID,
		},
	}

	res, err := client.GetItem(&dynamodb.GetItemInput{
		TableName:      &table,
		Key:            key,
		ConsistentRead: aws.Bool(true),
	})

	if err != nil {
		return errors.Wrap(err, "getting item")
	}

	if err := dynamodbattribute.UnmarshalMap(res.Item, &p); err != nil {
		return errors.Wrap(err, "unmarshaling item")
	}

	return nil
}

// Vote places a vote for `userID` against `option`.
// If the user has already voted then
// ErrAlreadyVoted is returned.
func (p *Poll) Vote(userID, option string) error {
	key := map[string]*dynamodb.AttributeValue{
		"id": {
			S: &p.ID,
		},
	}

	vals := map[string]*dynamodb.AttributeValue{
		":votes": {
			N: aws.String("1"),
		},
		":voter_set": {
			SS: aws.StringSlice([]string{userID}),
		},
		":voter": {
			S: &userID,
		},
	}

	names := map[string]*string{
		"#options": aws.String("options"),
		"#option":  &option,
	}

	_, err := client.UpdateItem(&dynamodb.UpdateItemInput{
		TableName:                 &table,
		Key:                       key,
		UpdateExpression:          aws.String(`ADD votes :votes, voters :voter_set SET #options.#option = #options.#option + :votes`),
		ConditionExpression:       aws.String(`not contains(voters, :voter)`),
		ExpressionAttributeValues: vals,
		ExpressionAttributeNames:  names,
	})

	if err != nil && strings.Contains(err.Error(), "ConditionalCheckFailedException") {
		return ErrAlreadyVoted
	}

	return err
}
