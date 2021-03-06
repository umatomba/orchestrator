/*
   Copyright 2015 Shlomi Noach, courtesy Booking.com

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package inst

import (
	"github.com/outbrain/golib/log"
	"github.com/outbrain/golib/sqlutils"
	"github.com/outbrain/orchestrator/config"
	"github.com/outbrain/orchestrator/db"
	"regexp"
)

// GetReplicationAnalysis will check for replication problems (dead master; unreachable master; etc)
func GetReplicationAnalysis__old() ([]ReplicationAnalysis, error) {
	result := []ReplicationAnalysis{}

	query := `
		SELECT * FROM (
		    SELECT 
			    hostname,
			    port,
			    cluster_name,
			    is_last_check_valid,
			    count_slaves,
			    count_valid_slaves,
			    count_valid_replicating_slaves,
			    CASE
		            WHEN 
		                    is_master
					        AND is_last_check_valid = 0
					        AND count_valid_slaves = count_slaves
					        AND count_valid_replicating_slaves = 0
		                THEN 
		                    'dead_master. This master cannot be reached by orchestrator and none of its slaves is replicating'
		            WHEN
					        is_master
					        AND count_slaves > 0
					        AND is_last_check_valid = 0
					        AND count_valid_slaves = 0
					        AND count_valid_replicating_slaves = 0
		                THEN
		                    'dead_master_and_slaves. This master cannot be reached by orchestrator; all of its slaves are unreachable'
		            WHEN
						    is_master
						    AND is_last_check_valid = 0
						    AND count_valid_slaves < count_slaves
						    AND count_valid_slaves > 0
						    AND count_valid_replicating_slaves = 0
		                THEN
		                    'dead_master_and_some_slaves. This master cannot be reached by orchestrator; some of its slaves are unreachable and none of its reachable slaves is replicating'            
		            WHEN
						    is_master
						    AND is_last_check_valid = 0
						    AND count_valid_slaves > 0
						    AND count_valid_replicating_slaves > 0
		                THEN
		                    'unreachable_master. This master cannot be reached by orchestrator but it has replicating slaves; possibly a network/host issue'
		            WHEN
						    is_master
						    AND is_last_check_valid = 1
						    AND count_slaves > 0
						    AND count_valid_replicating_slaves = 0
		                THEN
		                    'all_slaves_not_replicating. The master is reachable but none of its slaves is replicating'
		            WHEN
						    is_master
						    AND count_slaves = 0
		                THEN
		                    'master_without_slaves. The master does not have any slaves'
		        END as analysis
		    FROM (
				    SELECT 
				        master_instance.hostname,
				        master_instance.port,
				        MIN(master_instance.cluster_name) AS cluster_name,
				        MIN(master_instance.last_checked <= master_instance.last_seen)
				            IS TRUE AS is_last_check_valid,
				        MIN(master_instance.master_host IN ('' , '_')
				            OR master_instance.master_port = 0) AS is_master,
				        MIN(CONCAT(master_instance.hostname,
				                ':',
				                master_instance.port) = master_instance.cluster_name) AS is_cluster_master,
				        COUNT(slave_instance.server_id) AS count_slaves,
				        IFNULL(SUM(slave_instance.last_checked <= slave_instance.last_seen),
				                0) AS count_valid_slaves,
				        IFNULL(SUM(slave_instance.last_checked <= slave_instance.last_seen
				                    AND slave_instance.slave_io_running != 0
				                    AND slave_instance.slave_sql_running != 0),
				                0) AS count_valid_replicating_slaves
				    FROM
				        database_instance master_instance
				            LEFT JOIN
				        hostname_resolve ON (master_instance.hostname = hostname_resolve.hostname)
				            LEFT JOIN
				        database_instance slave_instance ON (COALESCE(hostname_resolve.resolved_hostname,
				                master_instance.hostname) = slave_instance.master_host
				            AND master_instance.port = slave_instance.master_port)
				    GROUP BY 
					    master_instance.hostname, 
					    master_instance.port
				    ORDER BY 
					    is_master DESC , 
					    is_cluster_master DESC, 
					    count_slaves DESC
		    ) select_summary
		) select_analysis
		WHERE analysis IS NOT NULL
	`
	db, err := db.OpenOrchestrator()
	if err != nil {
		goto Cleanup
	}

	err = sqlutils.QueryRowsMap(db, query, func(m sqlutils.RowMap) error {
		replicationAnalysis := ReplicationAnalysis{}

		replicationAnalysis.AnalyzedInstanceKey = InstanceKey{Hostname: m.GetString("hostname"), Port: m.GetInt("port")}
		replicationAnalysis.ClusterName = m.GetString("cluster_name")
		replicationAnalysis.LastCheckValid = m.GetBool("is_last_check_valid")
		replicationAnalysis.CountSlaves = m.GetUint("count_slaves")
		replicationAnalysis.CountValidSlaves = m.GetUint("count_valid_slaves")
		replicationAnalysis.CountValidReplicatingSlaves = m.GetUint("count_valid_replicating_slaves")
		//replicationAnalysis.Analysis = m.GetString("analysis")

		result = append(result, replicationAnalysis)
		return nil
	})
Cleanup:

	if err != nil {
		log.Errore(err)
	}
	return result, err

}

// GetReplicationAnalysis will check for replication problems (dead master; unreachable master; etc)
func GetReplicationAnalysis() ([]ReplicationAnalysis, error) {
	result := []ReplicationAnalysis{}

	query := `
		    SELECT 
		        master_instance.hostname,
		        master_instance.port,
		        MIN(master_instance.cluster_name) AS cluster_name,
		        MIN(master_instance.last_checked <= master_instance.last_seen)
		            IS TRUE AS is_last_check_valid,
		        MIN(master_instance.master_host IN ('' , '_')
		            OR master_instance.master_port = 0) AS is_master,
		        MIN(CONCAT(master_instance.hostname,
		                ':',
		                master_instance.port) = master_instance.cluster_name) AS is_cluster_master,
		        COUNT(slave_instance.server_id) AS count_slaves,
		        IFNULL(SUM(slave_instance.last_checked <= slave_instance.last_seen),
		                0) AS count_valid_slaves,
		        IFNULL(SUM(slave_instance.last_checked <= slave_instance.last_seen
		                    AND slave_instance.slave_io_running != 0
		                    AND slave_instance.slave_sql_running != 0),
		                0) AS count_valid_replicating_slaves
		    FROM
		        database_instance master_instance
		            LEFT JOIN
		        hostname_resolve ON (master_instance.hostname = hostname_resolve.hostname)
		            LEFT JOIN
		        database_instance slave_instance ON (COALESCE(hostname_resolve.resolved_hostname,
		                master_instance.hostname) = slave_instance.master_host
		            AND master_instance.port = slave_instance.master_port)
		    GROUP BY 
			    master_instance.hostname, 
			    master_instance.port
		    ORDER BY 
			    is_master DESC , 
			    is_cluster_master DESC, 
			    count_slaves DESC
	`
	db, err := db.OpenOrchestrator()
	if err != nil {
		goto Cleanup
	}

	err = sqlutils.QueryRowsMap(db, query, func(m sqlutils.RowMap) error {
		a := ReplicationAnalysis{Analysis: NoProblem}

		a.IsMaster = m.GetBool("is_master")
		a.AnalyzedInstanceKey = InstanceKey{Hostname: m.GetString("hostname"), Port: m.GetInt("port")}
		a.ClusterName = m.GetString("cluster_name")
		a.LastCheckValid = m.GetBool("is_last_check_valid")
		a.CountSlaves = m.GetUint("count_slaves")
		a.CountValidSlaves = m.GetUint("count_valid_slaves")
		a.CountValidReplicatingSlaves = m.GetUint("count_valid_replicating_slaves")

		if a.IsMaster && !a.LastCheckValid && a.CountSlaves == 0 {
			a.Analysis = DeadMasterWithoutSlaves
			a.Description = "Master cannot be reached by orchestrator and has no slave"
		} else if a.IsMaster && !a.LastCheckValid && a.CountValidSlaves == a.CountSlaves && a.CountValidReplicatingSlaves == 0 {
			a.Analysis = DeadMaster
			a.Description = "Master cannot be reached by orchestrator and none of its slaves is replicating"
		} else if a.IsMaster && !a.LastCheckValid && a.CountSlaves > 0 && a.CountValidSlaves == 0 && a.CountValidReplicatingSlaves == 0 {
			a.Analysis = DeadMasterAndSlaves
			a.Description = "Master cannot be reached by orchestrator and none of its slaves is replicating"
		} else if a.IsMaster && !a.LastCheckValid && a.CountValidSlaves < a.CountSlaves && a.CountValidSlaves > 0 && a.CountValidReplicatingSlaves == 0 {
			a.Analysis = DeadMasterAndSomeSlaves
			a.Description = "Master cannot be reached by orchestrator; some of its slaves are unreachable and none of its reachable slaves is replicating"
		} else if a.IsMaster && !a.LastCheckValid && a.CountValidSlaves > 0 && a.CountValidReplicatingSlaves > 0 {
			a.Analysis = UnreachableMaster
			a.Description = "Master cannot be reached by orchestrator but it has replicating slaves; possibly a network/host issue"
		} else if a.IsMaster && a.LastCheckValid && a.CountSlaves > 0 && a.CountValidReplicatingSlaves == 0 {
			a.Analysis = AllMasterSlavesNotReplicating
			a.Description = "Master is reachable but none of its slaves is replicating"
		} else if !a.IsMaster && !a.LastCheckValid && a.CountSlaves > 0 && a.CountValidSlaves == a.CountSlaves && a.CountValidReplicatingSlaves == 0 {
			a.Analysis = DeadIntermediateMaster
			a.Description = "Intermediate master cannot be reached by orchestrator and none of its slaves is replicating"
		} else if !a.IsMaster && !a.LastCheckValid && a.CountValidSlaves < a.CountSlaves && a.CountValidSlaves > 0 && a.CountValidReplicatingSlaves == 0 {
			a.Analysis = DeadIntermediateMasterAndSomeSlaves
			a.Description = "Intermediate master cannot be reached by orchestrator; some of its slaves are unreachable and none of its reachable slaves is replicating"
		} else if !a.IsMaster && !a.LastCheckValid && a.CountValidSlaves > 0 && a.CountValidReplicatingSlaves > 0 {
			a.Analysis = UnreachableIntermediateMaster
			a.Description = "Intermediate master cannot be reached by orchestrator but it has replicating slaves; possibly a network/host issue"
		} else if !a.IsMaster && a.LastCheckValid && a.CountSlaves > 0 && a.CountValidReplicatingSlaves == 0 {
			a.Analysis = AllIntermediateMasterSlavesNotReplicating
			a.Description = "Intermediate master is reachable but none of its slaves is replicating"
		}
		//		 else if a.IsMaster && a.CountSlaves == 0 {
		//			a.Analysis = MasterWithoutSlaves
		//			a.Description = "Master has no slaves"
		//		}

		if a.Analysis != NoProblem {
			skipThisHost := false
			for _, filter := range config.Config.RecoveryIgnoreHostnameFilters {
				if matched, _ := regexp.MatchString(filter, a.AnalyzedInstanceKey.Hostname); matched {
					skipThisHost = true
				}
			}
			if !skipThisHost {
				result = append(result, a)
			}
		}
		return nil
	})
Cleanup:

	if err != nil {
		log.Errore(err)
	}
	return result, err

}
