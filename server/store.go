package main

import (
	"database/sql"

	sq "github.com/Masterminds/squirrel"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	"github.com/mattermost/mattermost-server/v5/model"
)

type DBStore struct {
	conn *sql.DB
	sq   sq.StatementBuilderType
}

type ChannelData struct {
	Name        string
	DisplayName string
}

func NewDBStore(driverName, dataSource string) (*DBStore, error) {
	db, err := sql.Open(driverName, dataSource)
	if err != nil {
		return nil, err
	}

	builder := sq.StatementBuilder.PlaceholderFormat(sq.Question)
	if driverName == model.DATABASE_DRIVER_POSTGRES {
		builder = builder.PlaceholderFormat(sq.Dollar)
	}
	builder = builder.RunWith(db)

	return &DBStore{conn: db, sq: builder}, nil
}

func (db *DBStore) Close() {
	db.conn.Close()
}

func (db *DBStore) MostActiveChannels(userID, teamID string, activityThreshold int) ([]ChannelData, error) {
	myChannels, err := db.getMyChannelsForTeam(userID, teamID)
	if err != nil {
		return nil, err
	}

	lastWeek := model.GetMillis() - (int64(activityThreshold) * 60 * 1000)
	query := db.sq.Select("C.Name as Name, C.DisplayName as DisplayName").
		From("Posts AS P").
		Join("Channels AS C ON P.ChannelId = C.Id").
		Where(sq.Gt{"P.CreateAt": lastWeek}).
		Where(sq.Eq{"C.Type": model.CHANNEL_OPEN}).
		Where(sq.Eq{"C.TeamId": teamID}).
		Where(sq.Eq{"C.DeleteAt": 0}).
		Where(sq.NotEq{"P.ChannelId": myChannels}).
		GroupBy("C.Name, C.DisplayName").
		OrderBy("Count(P.Id) DESC").
		Limit(3)
	rows, err := query.Query()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	channels := []ChannelData{}
	for rows.Next() {
		var channel ChannelData
		if err := rows.Scan(&channel.Name, &channel.DisplayName); err != nil {
			return nil, err
		}
		channels = append(channels, channel)
	}
	return channels, nil
}

func (db *DBStore) MostPopulatedChannels(userID, teamID string) ([]ChannelData, error) {
	myChannels, err := db.getMyChannelsForTeam(userID, teamID)
	if err != nil {
		return nil, err
	}

	query := db.sq.Select("C.Name as Name, C.DisplayName as DisplayName").
		From("ChannelMembers AS CM").
		Join("Channels AS C ON CM.ChannelId = C.Id").
		Where(sq.Eq{"C.TeamId": teamID}).
		Where(sq.NotEq{"CM.UserId": userID}).
		Where(sq.NotEq{"CM.ChannelId": myChannels}).
		Where(sq.Eq{"C.DeleteAt": 0}).
		Where(sq.Eq{"C.Type": model.CHANNEL_OPEN}).
		GroupBy("C.Name, C.DisplayName").
		OrderBy("Count(CM.UserId) DESC").
		Limit(3)

	rows, err := query.Query()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	channels := []ChannelData{}
	for rows.Next() {
		var channel ChannelData
		if err := rows.Scan(&channel.Name, &channel.DisplayName); err != nil {
			return nil, err
		}
		channels = append(channels, channel)
	}
	return channels, nil
}

func (db *DBStore) getChannelMembers(channelID string) ([]string, error) {
	query := db.sq.Select("UserId").
		From("ChannelMembers").
		Where(sq.Eq{"ChannelId": channelID})

	rows, err := query.Query()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []string{}
	for rows.Next() {
		var user string
		if err := rows.Scan(&user); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}

func (db *DBStore) MostPopularChannelsByChannel(userID, channelID, teamID string) ([]ChannelData, error) {
	otherMembersInChannel, err := db.getChannelMembers(channelID)
	if err != nil {
		return nil, err
	}

	myChannels, err := db.getMyChannelsForTeam(userID, teamID)
	if err != nil {
		return nil, err
	}

	query := db.sq.Select("C.Name as Name, C.DisplayName as DisplayName").
		From("ChannelMembers AS CM").
		Join("Channels AS C ON CM.ChannelId = C.Id").
		Where(sq.Eq{"CM.UserId": otherMembersInChannel}).
		Where(sq.NotEq{"CM.ChannelId": myChannels}).
		Where(sq.Eq{"C.Type": model.CHANNEL_OPEN}).
		Where(sq.Eq{"C.TeamId": teamID}).
		Where(sq.Eq{"C.DeleteAt": 0}).
		GroupBy("C.Name, C.DisplayName").
		OrderBy("Count(CM.UserId) DESC").
		Limit(3)
	rows, err := query.Query()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	channels := []ChannelData{}
	for rows.Next() {
		var channel ChannelData
		if err := rows.Scan(&channel.Name, &channel.DisplayName); err != nil {
			return nil, err
		}
		channels = append(channels, channel)
	}
	return channels, nil
}

func (db *DBStore) getMyChannelsForTeam(userID string, teamID string) ([]string, error) {
	query := db.sq.Select("ChannelId").
		From("ChannelMembers AS CM").
		Join("Channels ON Channels.Id=CM.ChannelId").
		Where(sq.Eq{"UserId": userID}).
		Where(sq.Eq{"TeamId": teamID})
	rows, err := query.Query()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	channels := []string{}
	for rows.Next() {
		var channel string
		if err := rows.Scan(&channel); err != nil {
			return nil, err
		}
		channels = append(channels, channel)
	}
	return channels, nil
}

func (db *DBStore) getMyCoMembersForTeam(myChannels []string, userID string, teamID string) ([]string, error) {
	query := db.sq.Select("UserId").
		From("ChannelMembers AS CM").
		Join("Channels AS C ON CM.ChannelId=C.Id").
		Where(sq.Eq{"CM.ChannelId": myChannels}).
		Where(sq.NotEq{"C.Name": model.DEFAULT_CHANNEL}).
		Where(sq.NotEq{"CM.UserId": userID}).
		Distinct()

	rows, err := query.Query()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []string{}
	for rows.Next() {
		var user string
		if err := rows.Scan(&user); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}

func (db *DBStore) MostPopularChannelsByUserCoMembers(userID, teamID string) ([]ChannelData, error) {
	myChannels, err := db.getMyChannelsForTeam(userID, teamID)
	if err != nil {
		return nil, err
	}
	myCoMembers, err := db.getMyCoMembersForTeam(myChannels, userID, teamID)
	if err != nil {
		return nil, err
	}

	query := db.sq.Select("C.Name as Name, C.DisplayName as DisplayName").
		From("ChannelMembers AS CM").
		Join("Channels AS C ON CM.ChannelId = C.Id").
		Where(sq.Eq{"C.Type": model.CHANNEL_OPEN}).
		Where(sq.Eq{"C.TeamId": teamID}).
		Where(sq.Eq{"C.DeleteAt": 0}).
		Where(sq.Eq{"CM.UserId": myCoMembers}).
		Where(sq.NotEq{"CM.ChannelId": myChannels}).
		GroupBy("C.Name, C.DisplayName").
		OrderBy("Count(CM.UserId) DESC").
		Limit(3)

	rows, err := query.Query()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	channels := []ChannelData{}
	for rows.Next() {
		var channel ChannelData
		if err := rows.Scan(&channel.Name, &channel.DisplayName); err != nil {
			return nil, err
		}
		channels = append(channels, channel)
	}
	return channels, nil
}
