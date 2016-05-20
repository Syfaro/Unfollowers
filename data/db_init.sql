create table if not exists config (
	id integer primary key,

	key text unique not null,
	value text not null
);

create table if not exists tokens (
	id integer primary key,

	token text not null,
	secret text not null,

	twitter_id integer not null,
	screen_name text not null,
	display_name text not null
);

create table if not exists users (
	id integer primary key,

	twitter_id integer not null unique,
	screen_name text not null,
	display_name text not null,

	profile_icon text,
	color text
);

create table if not exists events (
	id integer primary key,

	token_id integer references tokens(id),
	user_id integer references user(id),

	event_type text check (event_type in ('f', 'u')),
	event_date datetime default current_timestamp
);
