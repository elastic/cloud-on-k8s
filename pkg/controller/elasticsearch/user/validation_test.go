// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package user

import "testing"

func Test_validPassword(t *testing.T) {
	type args struct {
		password []byte
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "nok: too short",
			args: args{
				password: []byte("u"),
			},
			wantErr: true,
		},
		{
			name: "ok",
			args: args{
				password: []byte("password"),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validPassword(tt.args.password); (err != nil) != tt.wantErr {
				t.Errorf("validPassword() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_validUserOrRoleName(t *testing.T) {
	type args struct {
		name string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "NOK too short",
			args: args{
				name: "",
			},
			wantErr: true,
		},
		{
			name: "NOK too long",
			args: args{
				name: "Lkb417z9pD1nf56ru5KtLAzX4WtEPipy3x4WOR1NsgUmrM2A7U1ZdWLSTzaUFsDyL15yI6j5o27o50c70XupgK5yrBrSnaboVRmJEjJ1UsYJXJ6U4lucMtfH25pXdjUmPtqFbDo7nS7UAMc97gJCDcFYeMUwSGSzksIDdwf4nM3XCqREErGrJB7gdIbQJSEFlr7rynkbNhu7e6yKg4dEdmOh4gzaAGWCagPsOkh4mDyEt9Lcob7eRrM1VUqlX1Q8OoPZedlFdAVrwjyuSMTYJZkuRAkgYpjo7pnMGTY8XsHwBL8a9C4SVg7PHjkh8CIIfIicsbKHmNjQFrMinvC3Hv67lNN6vOTvag9Pg47suTb9BV4tGYgFGORXkYfU8yy6ROjYRBy6jjW1UW4YNGpgw6KxAIcrX87ebBCE0FZwtr4aDUAyktSk13zBaYAUUxmexmIx6st02yfbQclUXe4BxrH8c6TIMzm1ASUevHMmSFsVKGLbWQbKqVOWTol1wNO9dKsRJZRHqvouT6TRldNeu4vdP1k8qaTtd2uy2wZ6cIrwWOgjERMfV40wO6vu5oFTfn5NgBrP5b6Ey3TJEsuzW7dXEaw6ge9ZWHcX22oxRUXmbEzZzaB4JQELNvREyeqYhb3KbetvuzpKuCselCkU6dJ5t5TX81itr5SSYRaGiOjoKMyYhNmpk5CEs7h0wJB8XQSKd6H02duF84PoYLm7pavxMTBmHQYUqXmLmYuAkT7YttCPCtTULgXvfTVD1znllZMVv0uKZUvNtBEThO8NKP3nfxQXTeEAOzkwrTMKp4QWdDLeanz4s78gIrUdSi0UFofm3mAWDeVHhnZwIuIoobrkxujgpRwH4QL4qqwgkEW5rYjeT0z0YRo3orStEft9mjVD8FQinQTDtukj5UfdYPSgJzNLGDrwE3MogPtZC3mz3zy6keBFLFKQXdyIA1qjpyiGIFFrA2uBQl3OYyN3v7pT1HgmxqEEMwuerH6NchLHAGJBdBg4aKP3jYUNSJvss",
			},
			wantErr: true,
		},
		{
			name: "NOK invalid char",
			args: args{
				name: "elästic",
			},
			wantErr: true,
		},
		{
			name: "NOK starts with whitespace",
			args: args{
				name: " elastic",
			},
			wantErr: true,
		},
		{
			name: "NOK ends with whitespace",
			args: args{
				name: "elastic ",
			},
			wantErr: true,
		},
		{
			name: "OK",
			args: args{
				name: "superuser",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validUserOrRoleName(tt.args.name); (err != nil) != tt.wantErr {
				t.Errorf("validUserOrRoleName() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_user_Validate(t *testing.T) {
	type fields struct {
		Name     string
		Password []byte
		Roles    []string
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "NOK invalid roles",
			fields: fields{
				Name:     "user",
				Password: []byte("secret"),
				Roles:    []string{"", "äußerung"},
			},
			wantErr: true,
		},
		{
			name: "NOK invalid user name",
			fields: fields{
				Name:     "",
				Password: []byte("secret"),
				Roles:    nil,
			},
			wantErr: true,
		},
		{
			name: "NOK invalid password",
			fields: fields{
				Name:     "elastic",
				Password: []byte("pass"),
				Roles:    nil,
			},
			wantErr: true,
		},
		{
			name: "OK",
			fields: fields{
				Name:     "elastic",
				Password: []byte("super-secret"),
				Roles:    []string{"superuser"},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := user{
				Name:     tt.fields.Name,
				Password: tt.fields.Password,
				Roles:    tt.fields.Roles,
			}
			if err := u.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
