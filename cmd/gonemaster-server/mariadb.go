package main

// Register the MySQL/MariaDB driver so that "mariadb" can be used as the
// database driver (the underlying sql.DB driver name is "mysql").
import _ "github.com/go-sql-driver/mysql"
