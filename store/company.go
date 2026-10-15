package store

import (
	"database/sql"
	"time"
)

type Company struct {
	Name                     string
	Address                  string
	VATNumber                string
	FPANumber                string
	AFTNumber                string
	Phone                    string
	Email                    string
	TechnicianName           string
	TechnicianSpeciality     string
	TechnicianRegistryNumber string
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

func GetCompany(db *sql.DB) (*Company, error) {
	if err := ensureCompany(db); err != nil {
		return nil, err
	}

	q := `SELECT name, address, vat_number, fpa_number, aft_number,
	             phone, email,
	             technician_name, technician_speciality,
	             technician_registry_number,
	             created_at, updated_at
	      FROM company_settings WHERE id = 1`

	var c Company
	var created, updated string
	err := db.QueryRow(q).Scan(
		&c.Name, &c.Address, &c.VATNumber,
		&c.FPANumber, &c.AFTNumber, &c.Phone,
		&c.Email, &c.TechnicianName,
		&c.TechnicianSpeciality, &c.TechnicianRegistryNumber,
		&created, &updated,
	)
	if err != nil {
		return nil, err
	}

	c.CreatedAt, _ = time.Parse(time.RFC3339, created)
	c.UpdatedAt, _ = time.Parse(time.RFC3339, updated)

	return &c, nil
}

func SaveCompany(db *sql.DB, c Company) error {
	now := time.Now().Format(time.RFC3339)
	q := `INSERT INTO company_settings
	      (id, name, address, vat_number, fpa_number, aft_number,
	       phone, email,
	       technician_name, technician_speciality,
	       technician_registry_number,
	       created_at, updated_at)
	      VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	      ON CONFLICT(id) DO UPDATE SET
	          name = excluded.name,
	          address = excluded.address,
	          vat_number = excluded.vat_number,
	          fpa_number = excluded.fpa_number,
	          aft_number = excluded.aft_number,
	          phone = excluded.phone,
	          email = excluded.email,
	          technician_name = excluded.technician_name,
	          technician_speciality = excluded.technician_speciality,
	          technician_registry_number =
	              excluded.technician_registry_number,
	          updated_at = excluded.updated_at`

	_, err := db.Exec(
		q, c.Name, c.Address, c.VATNumber,
		c.FPANumber, c.AFTNumber, c.Phone,
		c.Email, c.TechnicianName,
		c.TechnicianSpeciality, c.TechnicianRegistryNumber, now, now,
	)
	return err
}

func ensureCompany(db *sql.DB) error {
	now := time.Now().Format(time.RFC3339)
	q := `INSERT OR IGNORE INTO company_settings
	      (id, name, address, vat_number, fpa_number, aft_number,
	       phone, email,
	       technician_name, technician_speciality,
	       technician_registry_number,
	       created_at, updated_at)
	      VALUES (1, '', '', '', '', '', '', '', '', '', '', ?, ?)`
	_, err := db.Exec(q, now, now)
	return err
}
