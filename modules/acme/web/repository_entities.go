package web

import (
	"context"
	"time"
)

func (r *Repository) InitializeProviders(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS acme_providers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			directory_url TEXT NOT NULL,
			terms_of_service TEXT,
			website TEXT,
			status TEXT NOT NULL DEFAULT 'active',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_acme_providers_name ON acme_providers(name);
	`)
	return err
}

func (r *Repository) InitializeAccounts(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS acme_accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			provider_id INTEGER NOT NULL,
			account_url TEXT NOT NULL UNIQUE,
			email TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'valid',
			key_pem TEXT NOT NULL,
			thumbprint TEXT,
			agreed_terms BOOLEAN NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (provider_id) REFERENCES acme_providers(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_acme_accounts_provider_id ON acme_accounts(provider_id);
	`)
	return err
}

func (r *Repository) SeedProviders(ctx context.Context) error {
	defaults := []Provider{
		{Name: "Let's Encrypt", DirectoryURL: "https://acme-v02.api.letsencrypt.org/directory"},
		{Name: "Let's Encrypt Staging", DirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory"},
		{Name: "ZeroSSL", DirectoryURL: "https://acme.zerossl.com/v2/DV90"},
		{Name: "Buypass", DirectoryURL: "https://api.buypass.com/acme/directory"},
	}
	for _, provider := range defaults {
		if _, err := r.db.ExecContext(ctx, `INSERT OR IGNORE INTO acme_providers (name, directory_url) VALUES (?, ?)`, provider.Name, provider.DirectoryURL); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) CreateProvider(ctx context.Context, name, directoryURL string) (int64, error) {
	result, err := r.db.ExecContext(ctx, `INSERT INTO acme_providers (name, directory_url) VALUES (?, ?)`, name, directoryURL)
	if err != nil {
		return -1, err
	}
	return result.LastInsertId()
}

func (r *Repository) GetProvider(ctx context.Context, id int64) (*Provider, error) {
	var item Provider
	err := r.db.QueryRowContext(ctx, `SELECT id, name, directory_url, COALESCE(terms_of_service,''), COALESCE(website,''), status, created_at, updated_at FROM acme_providers WHERE id = ?`, id).Scan(
		&item.ID, &item.Name, &item.DirectoryURL, &item.TermsOfService, &item.Website, &item.Status, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *Repository) ListProviders(ctx context.Context) ([]Provider, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, name, directory_url, COALESCE(terms_of_service,''), COALESCE(website,''), status, created_at, updated_at FROM acme_providers ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Provider, 0)
	for rows.Next() {
		var item Provider
		if err := rows.Scan(&item.ID, &item.Name, &item.DirectoryURL, &item.TermsOfService, &item.Website, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) UpdateProvider(ctx context.Context, id int64, name, directoryURL, termsOfService, website string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE acme_providers SET name=?, directory_url=?, terms_of_service=?, website=?, updated_at=? WHERE id=?`, name, directoryURL, termsOfService, website, time.Now(), id)
	return err
}

func (r *Repository) DeleteProvider(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM acme_providers WHERE id=?`, id)
	return err
}

func (r *Repository) CreateAccount(ctx context.Context, providerID int64, accountURL, email, keyPEM, thumbprint string, agreedTerms bool) (*Account, error) {
	now := time.Now()
	result, err := r.db.ExecContext(ctx, `INSERT INTO acme_accounts (provider_id, account_url, email, status, key_pem, thumbprint, agreed_terms, created_at, updated_at) VALUES (?, ?, ?, 'valid', ?, ?, ?, ?, ?)`, providerID, accountURL, email, keyPEM, thumbprint, agreedTerms, now, now)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	provider, err := r.GetProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	return &Account{ID: id, ProviderID: providerID, ProviderName: provider.Name, AccountURL: accountURL, Email: email, Status: "valid", KeyPem: keyPEM, Thumbprint: thumbprint, AgreedTerms: agreedTerms, CreatedAt: now.Format("2006-01-02 15:04:05"), UpdatedAt: now.Format("2006-01-02 15:04:05")}, nil
}

func (r *Repository) GetAccount(ctx context.Context, id int64) (*Account, error) {
	var item Account
	err := r.db.QueryRowContext(ctx, `SELECT a.id,a.provider_id,COALESCE(p.name,''),a.account_url,a.email,a.status,a.key_pem,COALESCE(a.thumbprint,''),a.agreed_terms,a.created_at,a.updated_at FROM acme_accounts a LEFT JOIN acme_providers p ON p.id=a.provider_id WHERE a.id=?`, id).Scan(
		&item.ID, &item.ProviderID, &item.ProviderName, &item.AccountURL, &item.Email, &item.Status, &item.KeyPem, &item.Thumbprint, &item.AgreedTerms, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *Repository) ListAccounts(ctx context.Context) ([]Account, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT a.id,a.provider_id,COALESCE(p.name,''),a.account_url,a.email,a.status,a.key_pem,COALESCE(a.thumbprint,''),a.agreed_terms,a.created_at,a.updated_at FROM acme_accounts a LEFT JOIN acme_providers p ON p.id=a.provider_id ORDER BY a.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Account, 0)
	for rows.Next() {
		var item Account
		if err := rows.Scan(&item.ID, &item.ProviderID, &item.ProviderName, &item.AccountURL, &item.Email, &item.Status, &item.KeyPem, &item.Thumbprint, &item.AgreedTerms, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) UpdateAccount(ctx context.Context, id int64, accountURL, email, keyPEM, thumbprint string, agreedTerms bool) error {
	_, err := r.db.ExecContext(ctx, `UPDATE acme_accounts SET account_url=?, email=?, key_pem=?, thumbprint=?, agreed_terms=?, updated_at=? WHERE id=?`, accountURL, email, keyPEM, thumbprint, agreedTerms, time.Now(), id)
	return err
}

func (r *Repository) UpdateAccountStatus(ctx context.Context, id int64, status string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE acme_accounts SET status=?, updated_at=? WHERE id=?`, status, time.Now(), id)
	return err
}

func (r *Repository) DeleteAccount(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM acme_accounts WHERE id=?`, id)
	return err
}
