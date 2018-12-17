package main

import (
	"flag"
	"fmt"
	"log"
	"net"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/ec2metadata"
	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

type session struct {
	ec2 *ec2.EC2
}

func (s *session) setInstanceRoutingAttribute(instanceID string) error {
	req := s.ec2.ModifyInstanceAttributeRequest(&ec2.ModifyInstanceAttributeInput{
		InstanceId:      aws.String(instanceID),
		SourceDestCheck: &ec2.AttributeBooleanValue{Value: aws.Bool(false)},
	})
	_, err := req.Send()
	return err
}

func (s *session) setRoute(cidr, instanceID, routeTable string) error {
	exists, err := s.routeExists(cidr, routeTable)
	if err != nil {
		return err
	}

	if exists {
		return s.replaceRoute(cidr, instanceID, routeTable)
	}

	return s.createRoute(cidr, instanceID, routeTable)
}

func (s *session) routeExists(cidr, routeTable string) (bool, error) {
	describeReq := s.ec2.DescribeRouteTablesRequest(&ec2.DescribeRouteTablesInput{
		RouteTableIds: []string{routeTable},
	})
	describe, err := describeReq.Send()
	if err != nil {
		return false, err
	}
	if len(describe.RouteTables) != 1 {
		return false, fmt.Errorf("Expected 1 route table, got %d", len(describe.RouteTables))
	}

	routeTableInfo := describe.RouteTables[0]
	for _, route := range routeTableInfo.Routes {
		if *route.DestinationCidrBlock == cidr {
			return true, nil
		}
	}

	return false, nil
}

func (s *session) replaceRoute(cidr, instanceID, routeTable string) error {
	routeReq := s.ec2.ReplaceRouteRequest(&ec2.ReplaceRouteInput{
		DestinationCidrBlock: aws.String(cidr),
		InstanceId:           aws.String(instanceID),
		RouteTableId:         aws.String(routeTable),
	})
	_, err := routeReq.Send()
	return err
}

func (s *session) createRoute(cidr, instanceID, routeTable string) error {
	routeReq := s.ec2.CreateRouteRequest(&ec2.CreateRouteInput{
		DestinationCidrBlock: aws.String(cidr),
		InstanceId:           aws.String(instanceID),
		RouteTableId:         aws.String(routeTable),
	})
	_, err := routeReq.Send()
	return err
}

func main() {
	cidr := flag.String("cidr", "", "CIDR to route via this instance")
	routeTable := flag.String("rt", "", "route table ID to modify")
	flag.Parse()
	if *cidr == "" || *routeTable == "" {
		log.Fatalln("-cidr and -rt are mandatory flags")
	}

	_, ipNet, err := net.ParseCIDR(*cidr)
	if err != nil {
		log.Fatalln("Invalid CIDR:", err)
	}

	cfg, err := external.LoadDefaultAWSConfig()
	if err != nil {
		log.Fatalln("LoadDefaultAWSConfig failed:", err)
	}
	metadata := ec2metadata.New(cfg)
	doc, err := metadata.GetInstanceIdentityDocument()
	if err != nil {
		log.Fatalln("GetInstanceIdentityDocument failed:", err)
	}

	cfg.Region = doc.Region
	svc := ec2.New(cfg)
	s := &session{ec2: svc}

	err = s.setInstanceRoutingAttribute(doc.InstanceID)
	if err != nil {
		log.Fatalln("setInstanceRoutingAttribute failed:", err)
	}

	err = s.setRoute(ipNet.String(), doc.InstanceID, *routeTable)
	if err != nil {
		log.Fatalln("setRoute failed:", err)
	}
}
