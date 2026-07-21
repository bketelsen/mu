package auth

import (
	"fmt"
	"os"
	"sort"

	"mu/internal/data"
)

const singleOwnerMigrationVersion = 1

// AccountDeleteHook removes one account's data from a dependent package.
type AccountDeleteHook struct {
	Name   string
	Delete func(string) error
}

var accountDeleteHooks []AccountDeleteHook

// RegisterAccountDeleteHook registers a named synchronous account cleanup hook.
func RegisterAccountDeleteHook(name string, deleteFunc func(string) error) {
	accountDeleteHooks = append(accountDeleteHooks, AccountDeleteHook{Name: name, Delete: deleteFunc})
}

// MigrationResult describes a completed legacy-account migration.
type MigrationResult struct {
	OwnerID    string
	BackupPath string
	Deleted    int
	Reset      bool
	Migrated   bool
}

// MigrateSingleOwner converts legacy account state to one approved owner.
func MigrateSingleOwner(backup func() (string, error)) (MigrationResult, error) {
	var marker map[string]int
	if err := data.LoadJSON("single_owner_migration.json", &marker); err == nil {
		if marker["version"] == singleOwnerMigrationVersion {
			return MigrationResult{}, nil
		}
	} else if !os.IsNotExist(err) {
		return MigrationResult{}, fmt.Errorf("load migration marker: %w", err)
	}

	mutex.Lock()
	if len(accounts) == 0 {
		mutex.Unlock()
		return MigrationResult{}, nil
	}
	admins := make([]*Account, 0, len(accounts))
	deleteIDs := make([]string, 0, len(accounts))
	for id, account := range accounts {
		if id != "micro" && account.Admin {
			admins = append(admins, account)
		}
	}
	sort.Slice(admins, func(i, j int) bool {
		if admins[i].Created.Equal(admins[j].Created) {
			return admins[i].ID < admins[j].ID
		}
		return admins[i].Created.Before(admins[j].Created)
	})
	var survivor *Account
	if len(admins) > 0 {
		survivor = admins[0]
	}
	for id := range accounts {
		if survivor == nil || id != survivor.ID {
			deleteIDs = append(deleteIDs, id)
		}
	}
	mutex.Unlock()
	sort.Strings(deleteIDs)

	backupPath, err := backup()
	if err != nil {
		return MigrationResult{}, fmt.Errorf("backup data: %w", err)
	}
	for _, id := range deleteIDs {
		for _, hook := range accountDeleteHooks {
			if err := hook.Delete(id); err != nil {
				return MigrationResult{}, fmt.Errorf("cleanup %s for %s: %w", hook.Name, id, err)
			}
		}
		removeAccountCredentials(id)
	}
	if survivor != nil {
		mutex.Lock()
		survivor.Admin = true
		survivor.Approved = true
		survivor.Banned = false
		mutex.Unlock()
	}
	if err := deleteLegacyInvites(); err != nil {
		return MigrationResult{}, err
	}
	if err := persistAuthState(); err != nil {
		return MigrationResult{}, err
	}
	if err := data.SaveJSON("single_owner_migration.json", map[string]int{"version": singleOwnerMigrationVersion}); err != nil {
		return MigrationResult{}, err
	}
	result := MigrationResult{BackupPath: backupPath, Deleted: len(deleteIDs), Migrated: true}
	if survivor == nil {
		result.Reset = true
	} else {
		result.OwnerID = survivor.ID
	}
	return result, nil
}

func removeAccountCredentials(id string) {
	mutex.Lock()
	defer mutex.Unlock()
	delete(accounts, id)
	for sessionID, session := range sessions {
		if session.Account == id {
			delete(sessions, sessionID)
		}
	}
	for tokenID, token := range tokens {
		if token.Account == id {
			delete(tokens, tokenID)
		}
	}
	for passkeyID, passkey := range passkeys {
		if passkey.Account == id {
			delete(passkeys, passkeyID)
		}
	}
}

func persistAuthState() error {
	mutex.Lock()
	defer mutex.Unlock()
	if err := data.SaveJSON("accounts.json", accounts); err != nil {
		return err
	}
	if err := data.SaveJSON("sessions.json", sessions); err != nil {
		return err
	}
	if err := data.SaveJSON("tokens.json", tokens); err != nil {
		return err
	}
	return data.SaveJSON("passkeys.json", passkeys)
}

func deleteLegacyInvites() error {
	for _, key := range []string{"invites.json", "invite_requests.json"} {
		if err := data.DeleteFile(key); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
