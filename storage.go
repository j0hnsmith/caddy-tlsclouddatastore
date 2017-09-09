package tlsclouddatastore

import (
	"fmt"
	"net/url"
	"path"

	"os"

	"context"

	"time"

	"sync"

	"encoding/base64"

	"cloud.google.com/go/datastore"
	"github.com/mholt/caddy/caddytls"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

const (
	// DefaultPrefix defines the default prefix in KV store
	DefaultPrefix = "caddytls"

	// DefaultAESKeyB64 32 bytes when decoded
	DefaultAESKeyB64 = "Y29uc3VsdGxzLTEyMzQ1Njc4OTAtY2FkZHl0bHMtMzIK"

	// EnvNameAESKey defines the env variable name to override AES key, create with `openssl rand -base64 32` or similar
	EnvNameAESKey = "CADDY_CLOUDDATASTORETLS_B64_AESKEY"

	// EnvNamePrefix defines the env variable name to override key prefix
	EnvNamePrefix = "CADDY_CLOUDDATASTORETLS_PREFIX"

	EnvNameProjectId = "CADDY_CLOUDDATASTORETLS_PROJECT_ID" // id, not name

	// Create a service account at https://console.developers.google.com/permissions/serviceaccounts
	// with a Datastore -> Cloud Datastore User role, then create and download a json key for the service account.
	// This env var is the full path to the json key file
	EnvNameServiceAccountPath = "CADDY_CLOUDDATASTORETLS_SERVICE_ACCOUNT_FILE"

	SITE_RECORD = "caddytlsSiteRecord"
	USER_RECORD = "caddytlsUserRecord"
)

func init() {
	caddytls.RegisterStorageProvider("cloud-datastore", NewCloudDatastoreStorage)
}

// NewCloudDatastoreStorage connects to cloud datastore and returns a caddytls.Storage for the specific caURL
func NewCloudDatastoreStorage(caURL *url.URL) (caddytls.Storage, error) {

	ctx := context.Background()

	projectID := os.Getenv(EnvNameProjectId)
	if projectID == "" {
		return nil, fmt.Errorf("Unable read project id from env var: %s", EnvNameProjectId)
	}
	sAcctPath := os.Getenv(EnvNameServiceAccountPath)
	if sAcctPath == "" {
		return nil, fmt.Errorf("Unable read service account path from env var: %s", EnvNameServiceAccountPath)
	}

	var err error

	// Creates a client.
	cloudDsClient, err := datastore.NewClient(ctx, projectID, option.WithCredentialsFile(sAcctPath))
	if err != nil {
		return nil, fmt.Errorf("Unable to create Cloud Datastore client: %v", err)
	}

	cs := &CloudDsStorage{
		cloudDsClient: cloudDsClient,
		caHost:        caURL.Host,
		prefix:        DefaultPrefix,
		domainLocks:   make(map[string]*sync.WaitGroup),
	}

	k := DefaultAESKeyB64
	if aesKey := os.Getenv(EnvNameAESKey); aesKey != "" {
		k = aesKey
	}
	cs.aesKey, err = base64.StdEncoding.DecodeString(k)
	if err != nil {
		return nil, fmt.Errorf("Unable to decode AES key: %s", k)
	}

	if prefix := os.Getenv(EnvNamePrefix); prefix != "" {
		cs.prefix = prefix
	}

	return cs, nil
}

// CloudDsStorage holds all parameters for the Cloud Datastore connection
type CloudDsStorage struct {
	cloudDsClient *datastore.Client
	caHost        string
	prefix        string
	aesKey        []byte
	domainLocks   map[string]*sync.WaitGroup
	domainLocksMu sync.Mutex
}

type cdsEncryptedRecord struct {
	Value    []byte `datastore:",noindex"`
	Modified time.Time
}

type cdsEncryptedRecordWithLock struct {
	cdsEncryptedRecord
	Lock time.Time
}

func (cds *CloudDsStorage) key(suffix string) string {
	return path.Join(cds.prefix, cds.caHost, suffix)
}

func (cds *CloudDsStorage) siteKey(domain string) string {
	return cds.key(path.Join("sites", domain))
}

func (cds *CloudDsStorage) userKey(email string) string {
	return cds.key(path.Join("users", email))
}

// SiteExists checks if a cert for a specific domain already exists
func (cds *CloudDsStorage) SiteExists(domain string) (bool, error) {
	if _, err := cds.getSiteEntity(domain); err != nil {
		if err == datastore.ErrNoSuchEntity {
			// key doesn't exist
			return false, nil
		} else {
			// other unknown error
			return false, err
		}
	}

	return true, nil
}

// LoadSite loads the site data for a domain from Cloud Datastore
func (cds *CloudDsStorage) LoadSite(domain string) (*caddytls.SiteData, error) {
	r, err := cds.getSiteEntity(domain)
	if err != nil {
		return nil, fmt.Errorf("Unable to obtain site data for %v: %v", domain, err)
	}

	ret := new(caddytls.SiteData)
	if err := cds.fromBytes(r.Value, ret); err != nil {
		return nil, fmt.Errorf("Unable to decode site data for %v: %v", domain, err)
	}
	return ret, nil
}

// StoreSite stores the site data for a given domain in Cloud Datastore
func (cds *CloudDsStorage) StoreSite(domain string, data *caddytls.SiteData) error {
	r := new(cdsEncryptedRecordWithLock)
	var err error
	r.Value, err = cds.toBytes(data)
	r.Lock = time.Time{} // unset lock with nil value
	if err != nil {
		return fmt.Errorf("Unable to encode site data for %v: %v", domain, err)
	}

	if err := cds.putSiteEntity(domain, r); err != nil {
		return fmt.Errorf("Unable to store site data for %v: %v", domain, err)
	}

	return nil
}

// DeleteSite deletes site data for a given domain
func (cds *CloudDsStorage) DeleteSite(domain string) error {
	k := datastore.NameKey(SITE_RECORD, cds.siteKey(domain), nil)
	ctx := context.TODO()
	if err := cds.cloudDsClient.Delete(ctx, k); err != nil {
		return fmt.Errorf("Unable to delete site data for %v: %v", domain, err)
	}
	return nil
}

func (cds *CloudDsStorage) lockKey(domain string) string {
	return cds.key(path.Join("locks", domain))
}

// getSiteEntity gets an entity (the name for an object in Cloud Datastore parlance)
func (cds *CloudDsStorage) getSiteEntity(domain string) (*cdsEncryptedRecordWithLock, error) {
	k := datastore.NameKey(SITE_RECORD, cds.siteKey(domain), nil)
	ctx := context.TODO()
	r := new(cdsEncryptedRecordWithLock)
	err := cds.cloudDsClient.Get(ctx, k, r)
	return r, err
}

func (cds *CloudDsStorage) putSiteEntity(domain string, r *cdsEncryptedRecordWithLock) error {
	k := datastore.NameKey(SITE_RECORD, cds.siteKey(domain), nil)
	r.Modified = time.Now()

	ctx := context.TODO()
	_, err := cds.cloudDsClient.Put(ctx, k, r)
	return err
}

// TryLock attempts to set a global lock for a given domain. If a lock is
// already set it will return a `caddytls.Waiter` that will resolve when the lock is free.
func (cds *CloudDsStorage) TryLock(domain string) (caddytls.Waiter, error) {
	cds.domainLocksMu.Lock()
	defer cds.domainLocksMu.Unlock()
	wg, ok := cds.domainLocks[domain]
	if ok {
		// local lock already obtained, let caller wait on it
		return wg, nil
	}

	// no existing local lock, get the data so we can check if global lock
	r, err := cds.getSiteEntity(domain)

	if err != nil && err != datastore.ErrNoSuchEntity {
		return nil, fmt.Errorf("Unable to obtain site data for %v: %v", domain, err)
	}

	wg = new(sync.WaitGroup)
	wg.Add(1)
	cds.domainLocks[domain] = wg

	if time.Until(r.Lock).Nanoseconds() > 0 {
		// r.Lock is in the future, already locked globally

		go func() {
			// check on lock periodically
			for {
				select {
				case <-time.After(time.Duration(time.Millisecond * 250)):
					r, err := cds.getSiteEntity(domain)
					if err != nil {
						// can't return error to caller, all we can do is remove the local lock
						wg.Done()
						return
					}
					if time.Until(r.Lock).Nanoseconds() > 0 {
						// still locked
					} else {
						wg.Done()
						return
					}
				}
			}
		}()

		return wg, nil
	}

	// no existing global lock, create one
	r.Lock = time.Now().Add(time.Second * 30) // set global lock, time to renew cert before any other attempts

	if err := cds.putSiteEntity(domain, r); err != nil {
		return nil, fmt.Errorf("Unable to store site data for %v: %v", domain, err)
	}

	// new lock obtained
	return nil, nil
}

// Unlock releases an existing lock
func (cds *CloudDsStorage) Unlock(domain string) error {
	cds.domainLocksMu.Lock()
	defer cds.domainLocksMu.Unlock()

	r, err := cds.getSiteEntity(domain)
	if err != nil {
		return fmt.Errorf("Unable to obtain site data for %v: %v", domain, err)
	}
	if time.Until(r.Lock).Nanoseconds() > 0 {
		// this shouldn't happen as set in cds.StoreSite()
		r.Lock = time.Time{} // unset lock with nil value
		if err := cds.putSiteEntity(domain, r); err != nil {
			return fmt.Errorf("Unable to store site data for %v: %v", domain, err)
		}
	}

	wg, ok := cds.domainLocks[domain]
	if !ok {
		return fmt.Errorf("FileStorage: no lock to release for %s", domain)
	}
	wg.Done()
	delete(cds.domainLocks, domain)
	return nil
}

// LoadUser loads user data for a given email address
func (cds *CloudDsStorage) LoadUser(email string) (*caddytls.UserData, error) {
	k := datastore.NameKey(USER_RECORD, cds.userKey(email), nil)
	ctx := context.TODO()
	r := new(cdsEncryptedRecord)
	err := cds.cloudDsClient.Get(ctx, k, r)

	if err != nil {
		return nil, fmt.Errorf("Unable to obtain user data for %v: %v", email, err)
	}

	user := new(caddytls.UserData)
	if err := cds.fromBytes(r.Value, user); err != nil {
		return nil, fmt.Errorf("Unable to decode user data for %v: %v", email, err)
	}
	return user, nil
}

// StoreUser stores user data for a given email address in KV store
func (cds *CloudDsStorage) StoreUser(email string, data *caddytls.UserData) error {
	k := datastore.NameKey(USER_RECORD, cds.userKey(email), nil)
	r := new(cdsEncryptedRecord)
	r.Modified = time.Now()

	var err error
	if r.Value, err = cds.toBytes(data); err != nil {
		return fmt.Errorf("Unable to encode user data for %v: %v", email, err)
	}

	ctx := context.TODO()
	if _, err = cds.cloudDsClient.Put(ctx, k, r); err != nil {
		return fmt.Errorf("Unable to store user data for %v: %v", email, err)
	}

	return nil
}

// MostRecentUserEmail returns the last modified email address from cloud datastore
func (cds *CloudDsStorage) MostRecentUserEmail() string {
	email := ""
	q := datastore.NewQuery(USER_RECORD).
		Order("-Modified").
		Limit(1).
		KeysOnly()

	ctx := context.TODO()
	for it := cds.cloudDsClient.Run(ctx, q); ; {
		key, err := it.Next(nil)
		if err == iterator.Done {
			email = key.Name
			break
		}
		if err != nil {
			// no way of propagating error, what else can we do?
			return email
		}
	}

	return email
}
