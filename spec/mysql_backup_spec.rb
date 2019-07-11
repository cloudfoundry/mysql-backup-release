require 'rspec'
require 'json'
require 'yaml'
require 'bosh/template/test'

describe 'streaming-mysql-backup-client job' do
  let(:release) { Bosh::Template::Test::ReleaseDir.new(File.join(File.dirname(__FILE__), '..')) }
  let(:job) { release.job('streaming-mysql-backup-client') }
  let(:links) {[
    Bosh::Template::Test::Link.new(
      name: 'mysql-backup-tool',
      instances: [
        Bosh::Template::Test::LinkInstance.new(address: 'backup-instance-address-1'),
        Bosh::Template::Test::LinkInstance.new(address: 'backup-instance-address-2')
      ],
      properties: {
        'cf-mysql-backup' => {
          'endpoint_credentials' => {
            'username' => 'some-username',
            'password' => 'some-password'
          }
        }
      }
    )
  ]}

  describe 'streaming-mysql-backup-client template' do
    let(:template) { job.template('config/streaming-mysql-backup-client.yml') }
    context('when backup_local_node_only is true') do
       let(:spec) {{
        "cf-mysql-backup" => {
          "backup_local_node_only" => true,
          'symmetric_key' => 'some-symmetric-key',
          'tls' => {
            'ca_certificate' => 'some-ca'
          }
        }
       }}

       it 'use 127.0.0.1 as Ips in the config' do
         tpl_output = template.render(spec, consumes: links)
         tpl_yaml = YAML.load(tpl_output)
         expect(tpl_yaml['Ips'].size).to equal(1)
         expect(tpl_yaml['Ips']).to include("127.0.0.1")
       end
    end

    context('when backup_local_node_only is not set') do
       let(:spec) {{
        "cf-mysql-backup" => {
         'symmetric_key' => 'some-symmetric-key',
          'tls' => {
            'ca_certificate' => 'some-ca'
          }
        }
       }}

       it 'use backup tool links as Ips in the config' do
         tpl_output = template.render(spec, consumes: links)
         tpl_yaml = YAML.load(tpl_output)
         expect(tpl_yaml['Ips'].size).to equal(2)
         expect(tpl_yaml['Ips']).to include("backup-instance-address-1")
         expect(tpl_yaml['Ips']).to include("backup-instance-address-2")
       end
    end

  end
end
